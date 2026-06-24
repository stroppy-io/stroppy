package dsqgen

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stroppy-io/stroppy/third_party/gotpcds/dsdgen"
)

// cell is one evaluated value: either an integer or a string.
type cell struct {
	i     int64
	s     string
	isInt bool
}

func intCell(i int64) cell  { return cell{i: i, isInt: true} }
func strCell(s string) cell { return cell{s: s} }

func (c cell) str() string {
	if c.isInt {
		return strconv.FormatInt(c.i, 10)
	}
	return c.s
}

// evaluator produces parameter values for one query under one seed/scale.
type evaluator struct {
	rng   *rng
	scale float64
	dists *distCache
	sc    *dsdgen.Scaling
	env   map[string][]cell
}

func newEvaluator(seed int64, scale float64, dists *distCache) *evaluator {
	return &evaluator{
		rng:   newRNG(seed),
		scale: scale,
		dists: dists,
		sc:    dsdgen.NewScaling(scale),
		env:   map[string][]cell{},
	}
}

// evalTemplate evaluates every define in dependency order, returning name->values.
func (ev *evaluator) evalTemplate(t *Template) (map[string][]cell, error) {
	remaining := append([]Define(nil), t.Defines...)
	for len(remaining) > 0 {
		progress := false
		var next []Define
		for _, d := range remaining {
			if !ev.depsReady(d) {
				next = append(next, d)
				continue
			}
			vals, err := ev.define(d)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", t.Name, d.Name, err)
			}
			ev.env[d.Name] = vals
			progress = true
		}
		if !progress {
			return nil, fmt.Errorf("%s: unresolved define dependencies: %v", t.Name, names(next))
		}
		remaining = next
	}
	return ev.env, nil
}

func (ev *evaluator) depsReady(d Define) bool {
	for _, dep := range d.Deps {
		if _, ok := ev.env[dep]; !ok {
			return false
		}
	}
	return true
}

func names(ds []Define) []string {
	out := make([]string, len(ds))
	for i, d := range ds {
		out[i] = d.Name
	}
	return out
}

func (ev *evaluator) define(d Define) ([]cell, error) {
	if d.RHS.Kind == ExprCall && d.RHS.Func == "ulist" {
		return ev.ulist(d.RHS)
	}
	c, err := ev.expr(d.RHS)
	if err != nil {
		return nil, err
	}
	return []cell{c}, nil
}

func (ev *evaluator) expr(e Expr) (cell, error) {
	switch e.Kind {
	case ExprInt:
		return intCell(e.Int), nil
	case ExprStr:
		return strCell(e.Str), nil
	case ExprWord:
		return strCell(e.Word), nil
	case ExprRef:
		v, ok := ev.env[e.Word]
		if !ok || len(v) == 0 {
			return cell{}, fmt.Errorf("reference to unevaluated define %q", e.Word)
		}
		idx := 0
		if e.Idx > 0 {
			idx = e.Idx - 1
		}
		if idx >= len(v) {
			return cell{}, fmt.Errorf("define %q has %d values, want element %d", e.Word, len(v), e.Idx)
		}
		return v[idx], nil
	case ExprBinary:
		return ev.binary(e)
	case ExprTuple:
		if len(e.Args) == 0 {
			return cell{}, fmt.Errorf("empty tuple")
		}
		return ev.expr(e.Args[0]) // value; weight (args[1]) ignored
	case ExprCall:
		return ev.call(e)
	default:
		return cell{}, fmt.Errorf("unhandled expr kind %d", e.Kind)
	}
}

func (ev *evaluator) binary(e Expr) (cell, error) {
	l, err := ev.expr(e.Args[0])
	if err != nil {
		return cell{}, err
	}
	r, err := ev.expr(e.Args[1])
	if err != nil {
		return cell{}, err
	}
	if e.Word == "+" && (!l.isInt || !r.isInt) {
		return strCell(l.str() + r.str()), nil
	}
	if !l.isInt || !r.isInt {
		return cell{}, fmt.Errorf("non-integer operands to %q", e.Word)
	}
	switch e.Word {
	case "+":
		return intCell(l.i + r.i), nil
	case "-":
		return intCell(l.i - r.i), nil
	case "*":
		return intCell(l.i * r.i), nil
	case "/":
		if r.i == 0 {
			return cell{}, fmt.Errorf("division by zero")
		}
		return intCell(l.i / r.i), nil
	default:
		return cell{}, fmt.Errorf("unknown operator %q", e.Word)
	}
}

func (ev *evaluator) call(e Expr) (cell, error) {
	switch strings.ToLower(e.Func) {
	case "random":
		return ev.fnRandom(e)
	case "rowcount":
		return intCell(ev.rowcount(strOf(e.Args[0]))), nil
	case "distmember":
		return ev.fnDistmember(e)
	case "dist":
		return ev.fnDist(e)
	case "text":
		return ev.fnText(e)
	case "date":
		return ev.fnDate(e)
	case "ulist":
		vs, err := ev.ulist(e)
		if err != nil {
			return cell{}, err
		}
		return vs[0], nil
	default:
		return cell{}, fmt.Errorf("unsupported function %q", e.Func)
	}
}

func (ev *evaluator) fnRandom(e Expr) (cell, error) {
	if len(e.Args) < 2 {
		return cell{}, fmt.Errorf("random: need min,max")
	}
	min, err := ev.expr(e.Args[0])
	if err != nil {
		return cell{}, err
	}
	max, err := ev.expr(e.Args[1])
	if err != nil {
		return cell{}, err
	}
	return intCell(ev.rng.intn(min.i, max.i)), nil
}

func (ev *evaluator) fnDistmember(e Expr) (cell, error) {
	if len(e.Args) != 3 {
		return cell{}, fmt.Errorf("distmember: need name,index,column")
	}
	name, err := ev.nameOf(e.Args[0])
	if err != nil {
		return cell{}, err
	}
	idx, err := ev.expr(e.Args[1])
	if err != nil {
		return cell{}, err
	}
	col, err := ev.expr(e.Args[2])
	if err != nil {
		return cell{}, err
	}
	d, err := ev.dists.get(name)
	if err != nil {
		return cell{}, err
	}
	v, err := d.at(int(col.i), int(idx.i)-1) // index is 1-based in templates
	if err != nil {
		return cell{}, err
	}
	return strCell(v), nil
}

func (ev *evaluator) fnDist(e Expr) (cell, error) {
	if len(e.Args) < 2 {
		return cell{}, fmt.Errorf("dist: need name,column")
	}
	name, err := ev.nameOf(e.Args[0])
	if err != nil {
		return cell{}, err
	}
	col, err := ev.expr(e.Args[1])
	if err != nil {
		return cell{}, err
	}
	d, err := ev.dists.get(name)
	if err != nil {
		return cell{}, err
	}
	idx := int(ev.rng.intn(0, int64(d.size()-1)))
	v, err := d.at(int(col.i), idx)
	if err != nil {
		return cell{}, err
	}
	return strCell(v), nil
}

func (ev *evaluator) fnText(e Expr) (cell, error) {
	if len(e.Args) == 0 {
		return cell{}, fmt.Errorf("text: no choices")
	}
	pick := e.Args[ev.rng.intn(0, int64(len(e.Args)-1))]
	return ev.expr(pick) // tuple -> its value
}

func (ev *evaluator) fnDate(e Expr) (cell, error) {
	if len(e.Args) < 2 {
		return cell{}, fmt.Errorf("date: need begin,end")
	}
	begin, err := ev.expr(e.Args[0])
	if err != nil {
		return cell{}, err
	}
	end, err := ev.expr(e.Args[1])
	if err != nil {
		return cell{}, err
	}
	b, err := parseDate(begin.str())
	if err != nil {
		return cell{}, err
	}
	en, err := parseDate(end.str())
	if err != nil {
		return cell{}, err
	}
	days := int64(en.Sub(b).Hours() / 24)
	if days < 0 {
		days = 0
	}
	off := ev.rng.intn(0, days)
	return strCell(b.AddDate(0, 0, int(off)).Format("2006-01-02")), nil
}

// ulist evaluates ulist(expr, n) into n values, distinct where possible.
func (ev *evaluator) ulist(e Expr) ([]cell, error) {
	if len(e.Args) != 2 {
		return nil, fmt.Errorf("ulist: need expr,count")
	}
	n, err := ev.expr(e.Args[1])
	if err != nil {
		return nil, err
	}
	count := int(n.i)
	out := make([]cell, 0, count)
	seen := map[string]bool{}
	for attempts := 0; len(out) < count && attempts < count*50; attempts++ {
		c, err := ev.expr(e.Args[0])
		if err != nil {
			return nil, err
		}
		if seen[c.str()] {
			continue
		}
		seen[c.str()] = true
		out = append(out, c)
	}
	for len(out) < count { // domain too small for full distinctness; pad
		c, err := ev.expr(e.Args[0])
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// nameOf resolves an expression used as a distribution name (a bareword, a
// quoted string, or a nested call such as distmember(...) that yields a name).
func (ev *evaluator) nameOf(e Expr) (string, error) {
	if e.Kind == ExprWord {
		return e.Word, nil
	}
	if e.Kind == ExprStr {
		return e.Str, nil
	}
	c, err := ev.expr(e)
	if err != nil {
		return "", err
	}
	return c.str(), nil
}

func strOf(e Expr) string {
	switch e.Kind {
	case ExprStr:
		return e.Str
	case ExprWord:
		return e.Word
	default:
		return ""
	}
}

// rowcount approximates the qualification-database row count for a table or the
// size of a distribution-backed "active_*" subset. Exactness is not required:
// the value only bounds a random index that must land on a real row/member.
func (ev *evaluator) rowcount(name string) int64 {
	switch strings.ToLower(name) {
	case "store":
		return ev.sc.RowCount(dsdgen.TStore)
	case "store_sales":
		return ev.sc.RowCount(dsdgen.TStoreSales)
	case "item":
		return ev.sc.RowCount(dsdgen.TItem)
	case "customer":
		return ev.sc.RowCount(dsdgen.TCustomer)
	case "categories":
		return ev.distSize("categories", 30)
	case "active_counties":
		return ev.distSize("fips_county", 1000)
	case "active_cities":
		return ev.distSize("cities", 1000)
	case "active_states":
		return 50
	default:
		return 100
	}
}

func (ev *evaluator) distSize(name string, fallback int64) int64 {
	d, err := ev.dists.get(name)
	if err != nil {
		return fallback
	}
	return int64(d.size())
}

func parseDate(s string) (time.Time, error) {
	parts := strings.Split(strings.TrimSpace(s), "-")
	if len(parts) != 3 {
		return time.Time{}, fmt.Errorf("bad date %q", s)
	}
	y, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	d, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return time.Time{}, fmt.Errorf("bad date %q", s)
	}
	return time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC), nil
}
