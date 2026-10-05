package dbgen

const (
	rCmntSd  = 42
	rCmntLen = 72
)

type Region struct {
	Code    dssHuge
	Text    string
	Join    long
	Comment string
}

func (g *Generator) makeRegion(idx dssHuge) *Region {
	region := &Region{}

	region.Code = idx - 1
	region.Text = regions.members[idx-1].text
	region.Join = 0
	region.Comment = g.makeText(rCmntLen, rCmntSd)
	return region
}
