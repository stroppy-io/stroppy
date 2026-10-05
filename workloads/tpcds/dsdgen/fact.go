package dsdgen

// Fan-out fact-table engine for the sales/returns tables. Unlike the flat
// dimension tables (one output row per row number), a fact table is generated
// per "ticket/order": each order fans out to several line items, and the row
// number (ticket) only advances when an order completes. Each sales line may
// also emit a paired returns row. Mirrors the dsdgen sales row generators driven
// through the Results loop.
//
// Partition safety: a FactStream owns private sales and returns streamSets,
// seeked to the starting ticket. Per-ticket each stream consumes a fixed seed
// budget (consumeRemaining pads any slack at order end), so skip-ahead lands a
// partition on the same RNG state regardless of earlier tickets. Streams whose
// seedsPerRow is 0 (e.g. the item permutation) are therefore partition-invariant.

// FactLineGen produces one line item of the given ticket, advancing its own
// captured order state. It returns the sales row values and, when the line is
// returned, the paired returns row values (nil otherwise); endOrder reports
// whether this line completes the ticket.
type FactLineGen func(ticket int64) (sales []any, returns []any, endOrder bool)

// FactTable is one side (sales or returns) of a fan-out fact channel. Both sides
// share a channel's generation; emitReturns selects which rows this table emits.
type FactTable struct {
	Name        string
	ID          TableID
	Columns     []string
	emitReturns bool // false: sales (parent) rows; true: returns (child) rows
	salesCols   []GeneratorColumn
	returnsCols []GeneratorColumn
	// TicketCount returns the number of orders (the partition unit) at scale sf.
	TicketCount func(sf float64) int64
	// newLineGen builds a fresh, stateful line generator bound to this stream's
	// private sales/returns streamSets and scaling.
	newLineGen func(sss, srs *streamSet, sc *Scaling) FactLineGen
}

// FactStream emits the line-item rows for ticket range [start, start+count)
// (1-based; count<0 runs to the end). It buffers at most one pending row.
type FactStream struct {
	tbl    *FactTable
	sss    *streamSet
	srs    *streamSet
	gen    FactLineGen
	ticket int64
	end    int64
}

// NewStream builds a fact row source over ticket range [start, start+count) at
// scale sf. Both the sales and returns streamSets are seeked to the start ticket.
func (t *FactTable) NewStream(sf float64, start, count int64) *FactStream {
	end := start + count
	if count < 0 {
		end = t.TicketCount(sf) + 1
	}

	sss := newStreamSet(t.salesCols)
	srs := newStreamSet(t.returnsCols)
	sss.skipRows(start - 1)
	srs.skipRows(start - 1)

	return &FactStream{
		tbl:    t,
		sss:    sss,
		srs:    srs,
		gen:    t.newLineGen(sss, srs, NewScaling(sf)),
		ticket: start,
		end:    end,
	}
}

// Next returns the next emitted row (sales or returns depending on the table),
// or false at end of range. Returns rows that are not produced for a line (no
// return) are skipped transparently.
func (s *FactStream) Next() ([]any, bool) {
	for {
		if s.ticket >= s.end {
			return nil, false
		}

		sales, returns, endOrder := s.gen(s.ticket)

		out := sales
		if s.tbl.emitReturns {
			out = returns
		}

		if endOrder {
			s.sss.consumeRemaining()
			s.srs.consumeRemaining()
			s.ticket++
		}

		if out != nil {
			return out, true
		}
		// No row for this table on this line (a returns table line with no
		// return); advance to the next line.
	}
}
