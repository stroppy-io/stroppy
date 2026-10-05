package dbgen

import (
	"bytes"
	"fmt"
	"strings"
	"time"
)

const (
	startDate    = 92001
	currentDate  = 95168
	totDate      = 2557
	textPoolSize = 300 * 1024 * 1024
)

var szTextPool []byte

func makeAscDate() []string {
	var res []string
	date := time.Date(1992, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < totDate; i++ {
		newDate := date.AddDate(0, 0, i)
		ascDate := fmt.Sprintf("%4d-%02d-%02d", newDate.Year(), newDate.Month(), newDate.Day())
		res = append(res, ascDate)
	}
	return res
}

func makeSparse(idx dssHuge) dssHuge {
	return ((((idx >> 3) << 2) | (0 & 0x0003)) << 3) | (idx & 0x0007)
}

func (g *Generator) pickStr(dist *distribution, c int, target *string) (pos int) {
	j := long(g.random(1, dssHuge(dist.members[len(dist.members)-1].weight), long(c)))
	for pos = 0; dist.members[pos].weight < j; pos++ {
	}
	*target = dist.members[pos].text
	return
}

func (g *Generator) pickClerk() string {
	clkNum := g.random(1, max(dssHuge(scale*oClrkScl), oClrkScl), oClrkSd)
	return fmt.Sprintf("Clerk#%09d", clkNum)
}

func (g *Generator) txtVp(sd int) string {
	var src *distribution
	var syntax string
	var buf bytes.Buffer
	g.pickStr(&vp, sd, &syntax)

	for _, item := range strings.Split(syntax, " ") {
		switch item[0] {
		case 'D':
			src = &adverbs
		case 'V':
			src = &verbs
		case 'X':
			src = &auxillaries
		default:
			panic("unreachable")
		}
		var tmp string
		g.pickStr(src, sd, &tmp)
		buf.WriteString(tmp)
		if len(item) > 1 {
			buf.Write([]byte{item[1]})
		}

		buf.WriteString(" ")
	}

	return buf.String()
}

func (g *Generator) txtNp(sd int) string {
	var src *distribution
	var syntax string
	var buf bytes.Buffer
	g.pickStr(&np, sd, &syntax)

	for _, item := range strings.Split(syntax, " ") {
		switch item[0] {
		case 'A':
			src = &articles
		case 'J':
			src = &adjectives
		case 'D':
			src = &adverbs
		case 'N':
			src = &nouns
		default:
			panic("unreachable")
		}
		var tmp string
		g.pickStr(src, sd, &tmp)
		buf.WriteString(tmp)
		if len(item) > 1 {
			buf.Write([]byte{item[1]})
		}
		buf.WriteString(" ")
	}

	return buf.String()
}

func (g *Generator) txtSentence(sd int) string {
	var syntax string
	var buf bytes.Buffer
	g.pickStr(&grammar, sd, &syntax)

	for _, item := range strings.Split(syntax, " ") {
		switch item[0] {
		case 'V':
			buf.WriteString(g.txtVp(sd))
		case 'N':
			buf.WriteString(g.txtNp(sd))
		case 'P':
			var tmp string
			g.pickStr(&prepositions, sd, &tmp)
			buf.WriteString(tmp)
			buf.WriteString(" the ")
			buf.WriteString(g.txtNp(sd))
		case 'T':
			sentence := buf.String()
			sentence = sentence[0 : len(sentence)-1]
			buf.Reset()
			buf.WriteString(sentence)

			var tmp string
			g.pickStr(&terminators, sd, &tmp)
			buf.WriteString(tmp)
		default:
			panic("unreachable")
		}
		if len(item) > 1 {
			buf.Write([]byte{item[1]})
		}
	}
	return buf.String()
}

func (g *Generator) makeText(avg, sd int) string {
	min := int(float64(avg) * vStrLow)
	max := int(float64(avg) * vStrHgh)

	hgOffset := g.random(0, dssHuge(textPoolSize-max), long(sd))
	hgLength := g.random(dssHuge(min), dssHuge(max), long(sd))

	return string(szTextPool[hgOffset : hgOffset+hgLength])
}

func (g *Generator) aggStr(set *distribution, count, col long) string {
	var buf bytes.Buffer
	perm := g.permuteDist(set, col)

	for i := long(0); i < count; i++ {
		buf.WriteString(set.members[perm[i]].text)
		buf.WriteString(" ")
	}

	tmp := buf.String()
	return tmp[:len(tmp)-1]
}

func rpbRoutine(p dssHuge) dssHuge {
	price := dssHuge(90000)
	price += (p / 10) % 20001
	price += (p % 1000) * 100
	return price
}

func min(a, b dssHuge) dssHuge {
	if a < b {
		return a
	}
	return b
}
func max(a, b dssHuge) dssHuge {
	if a > b {
		return a
	}
	return b
}

func yeap(year int) int {
	if (year%4 == 0) && (year%100 != 0) {
		return 1
	}
	return 0
}

func julian(date int) int {
	offset := date - startDate
	result := startDate

	for true {
		yr := result / 1000
		yend := yr*1000 + 365 + yeap(yr)

		if result+offset > yend {
			offset -= yend - result + 1
			result += 1000
			continue
		} else {
			break
		}
	}
	return result + offset
}

func FmtMoney(m dssHuge) string {
	sign := ""
	if m < 0 {
		sign = "-"
		m = -m
	}
	return fmt.Sprintf("%s%d.%02d", sign, m/100, m%100)
}

func (g *Generator) sdNull(_ Table, _ dssHuge) {
}

// initTextPool builds the global read-only text pool using a THROWAWAY
// generator with fresh seeds. Upstream builds the pool from a fresh seed state
// right after initSeeds, so a throwaway Generator reproduces it identically.
func initTextPool() {
	tmp := &Generator{}
	tmp.initSeeds()

	var buffer bytes.Buffer

	for buffer.Len() < textPoolSize {
		sentence := tmp.txtSentence(5)
		len := len(sentence)

		needed := textPoolSize - buffer.Len()
		if needed >= len+1 {
			buffer.WriteString(sentence + " ")
		} else {
			buffer.WriteString(sentence[0:needed])
		}
	}

	szTextPool = buffer.Bytes()
}
