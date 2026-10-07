package dbgen

const (
	nCmntSd    = 41
	nCmntLen   = 72
	nationsMax = 90
)

type Nation struct {
	Code    dssHuge
	Text    string
	Join    long
	Comment string
}

func (g *Generator) makeNation(idx dssHuge) *Nation {
	nation := &Nation{}
	nation.Code = idx - 1
	nation.Text = nations.members[idx-1].text
	nation.Join = nations.members[idx-1].weight
	nation.Comment = g.makeText(nCmntLen, nCmntSd)

	return nation
}
