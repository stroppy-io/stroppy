package report

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const captureManifestBytes = 1 << 20

var ErrCaptureArtifact = errors.New("invalid capture artifact")

// CaptureRoot resolves a real UUID recording directory relative to a report.
func CaptureRoot(base string, capture *Capture) (*os.Root, error) {
	if capture == nil || capture.Schema != CaptureSchema {
		return nil, ErrCaptureArtifact
	}

	id, err := uuid.Parse(capture.ID)
	if err != nil || id.String() != capture.ID {
		return nil, ErrCaptureArtifact
	}

	directory := capture.Directory
	if !filepath.IsAbs(directory) {
		if directory != "captures/"+capture.ID {
			return nil, ErrCaptureArtifact
		}

		directory = filepath.Join(base, filepath.FromSlash(directory))
	}

	if filepath.Base(filepath.Clean(directory)) != capture.ID {
		return nil, ErrCaptureArtifact
	}

	return openHistory(directory)
}

// OpenCaptureFile opens only declared regular files and rejects replacement links.
func OpenCaptureFile(base string, capture *Capture, name string) (*os.File, error) {
	if capture == nil {
		return nil, ErrCaptureArtifact
	}

	if name != "manifest.json" {
		key := strings.TrimSuffix(name, ".jsonl")

		stream, ok := capture.Streams[key]
		if !ok || !stream.Enabled || stream.File != name || (key != "logs" && key != "metrics" && key != "events") {
			return nil, ErrCaptureArtifact
		}
	}

	root, err := CaptureRoot(base, capture)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}

	if !info.Mode().IsRegular() {
		return nil, ErrCaptureArtifact
	}

	file, err := openHistoryReport(root, name)
	if err != nil {
		return nil, err
	}

	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		file.Close()

		return nil, errors.Join(ErrCaptureArtifact, err)
	}

	return file, nil
}

// ReadCaptureFile retrieves a complete declared recording artifact.
func ReadCaptureFile(base string, capture *Capture, name string) ([]byte, error) {
	file, err := OpenCaptureFile(base, capture, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}

// AttachCapture copies a recording beside a report with an exclusively owned storage identity.
//

func AttachCapture(run *Run, base, destination string) (result *Run, err error) {
	if run.Capture == nil {
		return run, nil
	}

	sourceRoot, err := CaptureRoot(base, run.Capture)
	if err != nil {
		return nil, err
	}

	sourceRoot.Close()

	id := uuid.NewString()
	relative := filepath.Join("captures", id)

	target := filepath.Join(destination, relative)
	if err := os.MkdirAll(filepath.Dir(target), historyDirPerm); err != nil {
		return nil, err
	}

	if err := os.Mkdir(target, historyDirPerm); err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, os.RemoveAll(target))
		}
	}()

	attached := *run.Capture

	attached.Owner = "history"

	attached.SourceID = run.Capture.SourceID
	if attached.SourceID == "" {
		attached.SourceID = run.Capture.ID
	}

	attached.ID = id
	attached.Directory = filepath.ToSlash(relative)
	result = &Run{}
	*result = *run

	result.Capture = &attached
	for _, name := range captureNames(&attached) {
		if name == "manifest.json" {
			continue
		}

		source, openErr := OpenCaptureFile(base, run.Capture, name)
		if openErr != nil {
			return nil, openErr
		}

		output, openErr := os.OpenFile(filepath.Join(target, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, historyFilePerm)
		if openErr != nil {
			source.Close()

			return nil, openErr
		}

		_, copyErr := io.Copy(output, source)
		if closeErr := errors.Join(copyErr, source.Close(), output.Close()); closeErr != nil {
			return nil, closeErr
		}
	}

	data, err := json.MarshalIndent(&attached, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(filepath.Join(target, "manifest.json"), append(data, '\n'), historyFilePerm); err != nil {
		return nil, err
	}

	return result, nil
}

// OpenArtifact retrieves a declared artifact by history identity without buffering it.
func (h *History) OpenArtifact(id, name string) (*os.File, error) {
	data, err := h.Get(id)
	if err != nil {
		return nil, err
	}

	capture, err := CaptureFromJSON(data)
	if err != nil {
		return nil, err
	}

	return OpenCaptureFile(h.Directory, capture, name)
}

// Artifact retrieves a complete artifact as data for programmatic callers.
func (h *History) Artifact(id, name string) ([]byte, error) {
	file, err := h.OpenArtifact(id, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}

// WriteBundle streams complete JSON and artifacts to a portable ZIP.
func WriteBundle(output io.Writer, data []byte, base string) error {
	archive := zip.NewWriter(output)

	return errors.Join(WriteBundleInto(archive, data, base), archive.Close())
}

// WriteBundleInto appends portable report JSON and complete streams to an owned ZIP writer.
func WriteBundleInto(archive *zip.Writer, data []byte, base string) error {
	capture, err := CaptureFromJSON(data)
	if err != nil {
		return err
	}

	portable, err := bundleJSON(data, capture)
	if err != nil {
		return err
	}

	writer, err := archive.Create("report.json")
	if err != nil {
		return err
	}

	if _, err = writer.Write(portable); err != nil {
		return err
	}

	if capture != nil {
		for _, name := range captureNames(capture) {
			file, openErr := OpenCaptureFile(base, capture, name)
			if openErr != nil {
				return openErr
			}

			path := filepath.ToSlash(filepath.Join("captures", capture.ID, name))

			writer, err = archive.Create(path)
			if err == nil {
				_, err = io.Copy(writer, file)
			}

			err = errors.Join(err, file.Close())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// PortableCaptureJSON preserves report fields while making recording references relative.
func PortableCaptureJSON(data []byte) ([]byte, error) {
	capture, err := CaptureFromJSON(data)
	if err != nil {
		return nil, err
	}

	return bundleJSON(data, capture)
}

func bundleJSON(data []byte, capture *Capture) ([]byte, error) {
	if capture == nil {
		return data, nil
	}

	copied := *capture
	copied.Directory = "captures/" + capture.ID

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, err
	}

	encoded, err := json.Marshal(copied)
	if err != nil {
		return nil, err
	}

	envelope["capture"] = encoded

	return json.MarshalIndent(envelope, "", "  ")
}

// Bundle exports saved JSON and its complete capture streams.
func (h *History) Bundle(output io.Writer, id string) error {
	data, err := h.Get(id)
	if err != nil {
		return err
	}

	return WriteBundle(output, data, h.Directory)
}

func captureNames(c *Capture) []string {
	names := []string{"manifest.json"}

	for _, name := range []string{"events", "metrics", "logs"} {
		if c.Streams[name].Enabled {
			names = append(names, name+".jsonl")
		}
	}

	return names
}

//nolint:cyclop,gocognit // verify every owned file before deleting any artifact.
func removeCapture(root *os.Root, data []byte) error {
	c, err := CaptureFromJSON(data)
	if err != nil || c == nil {
		return nil
	}

	if c.Owner != "history" {
		return nil
	}

	id, err := uuid.Parse(c.ID)
	if err != nil || id.String() != c.ID || c.Directory != "captures/"+c.ID {
		return ErrCaptureArtifact
	}

	directoryInfo, err := root.Lstat(c.Directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}

	if err != nil || !directoryInfo.IsDir() || directoryInfo.Mode()&os.ModeSymlink != 0 {
		return errors.Join(ErrCaptureArtifact, err)
	}

	child, err := root.OpenRoot(c.Directory)
	if err != nil {
		return err
	}
	defer child.Close()

	opened, err := child.Stat(".")
	if err != nil || !os.SameFile(directoryInfo, opened) {
		return errors.Join(ErrCaptureArtifact, err)
	}

	names, err := fs.ReadDir(child.FS(), ".")
	if err != nil {
		return err
	}

	for _, entry := range names {
		name := entry.Name()
		if name != "manifest.json" && name != "logs.jsonl" && name != "metrics.jsonl" && name != "events.jsonl" {
			return ErrCaptureArtifact
		}

		info, err := child.Lstat(name)
		if err != nil || !info.Mode().IsRegular() {
			return errors.Join(ErrCaptureArtifact, err)
		}
	}

	for _, entry := range names {
		if err := child.Remove(entry.Name()); err != nil {
			return err
		}
	}

	return root.Remove(c.Directory)
}

// RemoveCapture removes only the declared UUID directory and known recording files.
func RemoveCapture(base string, c *Capture) error {
	if c == nil {
		return nil
	}

	root, err := CaptureRoot(base, c)
	if err != nil {
		return err
	}

	directory := root.Name()
	root.Close()

	parent, err := openHistory(filepath.Dir(filepath.Dir(directory)))
	if err != nil {
		return err
	}
	defer parent.Close()

	owned := *c
	owned.Owner = "history"
	owned.Directory = "captures/" + c.ID

	if filepath.Base(filepath.Dir(directory)) != "captures" {
		return ErrCaptureArtifact
	}

	data, err := json.Marshal(struct {
		Capture *Capture `json:"capture"`
	}{&owned})
	if err != nil {
		return err
	}

	return removeCapture(parent, data)
}

// OutputOverwritesCapture rejects destinations aliased to recording data.
func OutputOverwritesCapture(path, base string, c *Capture) (bool, error) {
	if c == nil {
		return false, nil
	}

	root, err := CaptureRoot(base, c)
	if err != nil {
		return false, err
	}

	directory := root.Name()
	root.Close()

	for _, name := range captureNames(c) {
		same, err := SameOutput(path, filepath.Join(directory, name))
		if err != nil || same {
			return same, err
		}
	}

	return false, nil
}

// CaptureFromJSON decodes only the optional recording envelope.
func CaptureFromJSON(data []byte) (*Capture, error) {
	var envelope struct {
		Capture *Capture `json:"capture"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("capture envelope: %w", err)
	}

	return envelope.Capture, nil
}

// CaptureState distinguishes interrupted and missing recordings from final reports.
func CaptureState(base string, c *Capture) string {
	if c == nil {
		return ""
	}

	file, err := OpenCaptureFile(base, c, "manifest.json")
	if err != nil {
		return "missing"
	}
	defer file.Close()

	var disk Capture
	if json.NewDecoder(io.LimitReader(file, captureManifestBytes)).Decode(&disk) != nil || disk.ID != c.ID {
		return "unreadable"
	}

	if disk.State == "recording" {
		return "incomplete"
	}

	return disk.State
}
