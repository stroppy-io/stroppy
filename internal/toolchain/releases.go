package toolchain

import (
	"fmt"
	"runtime"
)

const downloadBaseURL = "https://go.dev/dl/"

type release struct {
	File   string
	SHA256 string
}

var releases = map[string]release{
	"aix/ppc64": {
		"go1.27.4.aix-ppc64.tar.gz",
		"338a4830bfb09dcfc89d5ec80610d39f50305490ee0402826519cb3f5e3f783b",
	},
	"darwin/amd64": {
		"go1.27.4.darwin-amd64.tar.gz",
		"2b799377704ef12db39bd254995d5c348c9cc8d86562ca817724390e56c38f43",
	},
	"darwin/arm64": {
		"go1.27.4.darwin-arm64.tar.gz",
		"91a44a2b8a716600ad94a4f143e156116620db8828086a6e3602291be9bd0bb4",
	},
	"dragonfly/amd64": {
		"go1.27.4.dragonfly-amd64.tar.gz",
		"0d8ba3b49cd49bc2b482512647d851aa157a2d2924ecf6c371185de8e5ec5a06",
	},
	"freebsd/386": {
		"go1.27.4.freebsd-386.tar.gz",
		"a9ab56efddcbfbb27cf4a59264e8fe0ac1612884f9fa84155b833cad73f58529",
	},
	"freebsd/amd64": {
		"go1.27.4.freebsd-amd64.tar.gz",
		"063e079bf195da4bc2a1f424be26f20a57d64e0b7cc951cab943e6341a4ec0d3",
	},
	"freebsd/arm": {
		"go1.27.4.freebsd-arm.tar.gz",
		"3b1b4b0bf4848bf2d95ce02d3fba9fc2c4e7801ec8b1441d19d50b8c446cca76",
	},
	"freebsd/arm64": {
		"go1.27.4.freebsd-arm64.tar.gz",
		"73a3ee5f5e17a67d3ae7ccd8d06c6d4e7d59cfd1bed86427bf2d42c0bd3a6701",
	},
	"illumos/amd64": {
		"go1.27.4.illumos-amd64.tar.gz",
		"e0607e33048ba0ac36be1e423e502bcc2eafb4a4c164aa1d134bf0bf1e92f19e",
	},
	"linux/386": {
		"go1.27.4.linux-386.tar.gz",
		"6ecb45737d2315acb0dbbc97ea8e13e7c19d4c796abe8d68907c40bce6f50bf7",
	},
	"linux/amd64": {
		"go1.27.4.linux-amd64.tar.gz",
		"7226736c2339719cda28fbf3a54d71aa63bcefe376e98cef00034268dab24840",
	},
	"linux/arm64": {
		"go1.27.4.linux-arm64.tar.gz",
		"3392582988a273a87ab5043ee1b2c9c17a2c3c00580bf9326c47a2e0aaaeac92",
	},
	"linux/arm": {
		"go1.27.4.linux-armv6l.tar.gz",
		"e50964c41c6e187375387939e2f380de7250eba44e2259dfeb75c540d9d585c3",
	},
	"linux/loong64": {
		"go1.27.4.linux-loong64.tar.gz",
		"e484849e103aa464e4f8dd0ab7fe174a7948c50b6156aaf24d39b25841e276dd",
	},
	"linux/mips": {
		"go1.27.4.linux-mips.tar.gz",
		"31eb199b46b91ee1f416f5a2bdf52b94146c59fb9f7600551158042f3e12d61e",
	},
	"linux/mips64": {
		"go1.27.4.linux-mips64.tar.gz",
		"019bbe71ab71b7ddbf352a2ae5616c644d5a583df541c16f575142558fd93e8c",
	},
	"linux/mips64le": {
		"go1.27.4.linux-mips64le.tar.gz",
		"99516595926eb3d17678795c0840234808491b3b21d383bd3df318b02629b1e1",
	},
	"linux/mipsle": {
		"go1.27.4.linux-mipsle.tar.gz",
		"4b9609edf06a9ce7a495f5ca402f86ee557e5a3de00a135ac9a5a9e458d52881",
	},
	"linux/ppc64": {
		"go1.27.4.linux-ppc64.tar.gz",
		"d03699175326d176e920d5165bc4ab274ef2311f97d5180ffc9c0d82403a650c",
	},
	"linux/ppc64le": {
		"go1.27.4.linux-ppc64le.tar.gz",
		"f508314644fa6f466d1935f8a34f01e7980092a4730bfb9d7fa3ffde81910d20",
	},
	"linux/riscv64": {
		"go1.27.4.linux-riscv64.tar.gz",
		"19e9c376178180588954a896baca95f8baa03b69216f18316af4ee2c251cc623",
	},
	"linux/s390x": {
		"go1.27.4.linux-s390x.tar.gz",
		"4e56b1b9f26c649c1f3dc61fd16443d0154bd0860e59a8032b4cb05ac42f283e",
	},
	"netbsd/386": {
		"go1.27.4.netbsd-386.tar.gz",
		"8209f9a102edc9f7049425811345d75692214c8801c482bae9de35cbf50060e6",
	},
	"netbsd/amd64": {
		"go1.27.4.netbsd-amd64.tar.gz",
		"ba4f547835ed302e9d5e376fa63246a7a6c32af9576778135d5d680205a91472",
	},
	"netbsd/arm": {
		"go1.27.4.netbsd-arm.tar.gz",
		"1ea0b1344b6716b632835dc44da30b729cb479ec9ba9d7b6d68232e0d506dbf5",
	},
	"netbsd/arm64": {
		"go1.27.4.netbsd-arm64.tar.gz",
		"6f5f8d77225c91d3209b4a3565da5e3742798fdfa4024dca200c45fdd37a52a9",
	},
	"openbsd/386": {
		"go1.27.4.openbsd-386.tar.gz",
		"4e355307663a9e6d56749d1833f5d1745222ffc58d2e7c84581c9a6422fc1719",
	},
	"openbsd/amd64": {
		"go1.27.4.openbsd-amd64.tar.gz",
		"cb3b5da893614c4814c096052d7bc538d6743a33ddee3df06e060e1d3a7f7020",
	},
	"openbsd/arm": {
		"go1.27.4.openbsd-arm.tar.gz",
		"6d39928b44063c54f152cf4ed07d17dd51493f8ff2d5aa1deff0998d07772330",
	},
	"openbsd/arm64": {
		"go1.27.4.openbsd-arm64.tar.gz",
		"fc7e05cbfcf2a1392d1e268107b14b1e37a1972ab9cbc42ef46036ba9103fe9c",
	},
	"openbsd/ppc64": {
		"go1.27.4.openbsd-ppc64.tar.gz",
		"d0fd121ab9f641ae5d8e3f93002f83c90ec0abf97f03b5e9685a2a2f8c921f78",
	},
	"openbsd/riscv64": {
		"go1.27.4.openbsd-riscv64.tar.gz",
		"7db3d541301cf18ce5da4128a8a7f8b7a68cbc72f749d7e9a953efefdbd03431",
	},
	"solaris/amd64": {
		"go1.27.4.solaris-amd64.tar.gz",
		"5db6fa8cbdb19cdbf31be6f1f871716e6a763070ecf210e0fbc9e182ab9749ab",
	},
}

func hostRelease() (release, error) {
	target := runtime.GOOS + "/" + runtime.GOARCH

	value, ok := releases[target]
	if !ok {
		return release{}, fmt.Errorf("%w: %s", ErrUnsupportedHost, target)
	}

	return value, nil
}
