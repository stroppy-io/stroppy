package dsdgen

// ShipMode column stream layout (table-local indices into the streamSet). Global
// column numbers and per-row seed counts come from ShipModeGeneratorColumn.java.
const (
	smShipModeSk = iota
	smShipModeID
	smType
	smCode
	smContract
	smCarrier
	smNulls
)

var shipModeCols = []GeneratorColumn{
	smShipModeSk: {GlobalColumnNumber: 252, SeedsPerRow: 1},
	smShipModeID: {GlobalColumnNumber: 253, SeedsPerRow: 1},
	smType:       {GlobalColumnNumber: 254, SeedsPerRow: 1},
	smCode:       {GlobalColumnNumber: 255, SeedsPerRow: 1},
	smContract:   {GlobalColumnNumber: 256, SeedsPerRow: 21},
	smCarrier:    {GlobalColumnNumber: 257, SeedsPerRow: 1},
	smNulls:      {GlobalColumnNumber: 258, SeedsPerRow: 2},
}

// Ship-mode distribution tables (built once, read-only).
var (
	shipModeCarrierDist = mustLoadStringValues("ship_mode_carrier.dst", 1, 1)
	shipModeCodeDist    = mustLoadStringValues("ship_mode_code.dst", 1, 1)
	shipModeTypeDist    = mustLoadStringValues("ship_mode_type.dst", 1, 1)
)

// alphaNumeric is the character set used for sm_contract, from
// RandomValueGenerator.ALPHA_NUMERIC. Note the omitted 'w'/'W' (matches dsdgen).
const alphaNumeric = "abcdefghijklmnopqrstuvxyzABCDEFGHIJKLMNOPQRSTUVXYZ0123456789"

// generateRandomCharset mirrors RandomValueGenerator.generateRandomCharset: it
// draws a length in [min,max], then always draws max char indices (consuming a
// fixed number of seeds) but only appends the first length of them.
func generateRandomCharset(set string, min, max int, s *RNStream) string {
	length := GenerateUniformRandomInt(min, max, s)
	out := make([]byte, 0, max)
	for i := 0; i < max; i++ {
		index := GenerateUniformRandomInt(0, len(set)-1, s)
		if i < length {
			out = append(out, set[index])
		}
	}

	return string(out)
}

// valueForIndexModSize returns the value at index%Size, mirroring
// DistributionUtils.getValueForIndexModSize.
func valueForIndexModSize(d *StringValuesDistribution, index int64) string {
	return d.ValueAtIndex(0, int(index%int64(d.Size())))
}

// ShipMode is the TPC-DS ship_mode table. It is flat and fixed-size (20 rows at
// every scale). nullBasisPoints is 0 so no column is ever nulled, but SM_NULLS
// still consumes its two draws per row to keep stream alignment identical to
// dsdgen.
var ShipMode = &Table{
	Name:     "ship_mode",
	Columns:  []string{"sm_ship_mode_sk", "sm_ship_mode_id", "sm_type", "sm_code", "sm_carrier", "sm_contract"},
	Cols:     shipModeCols,
	RowCount: func(float64) int64 { return 20 },
	Row: func(rowNumber int64, ss *streamSet) []any {
		CreateNullBitMap(0, 0x03, ss.at(smNulls))

		smTypeVal := valueForIndexModSize(shipModeTypeDist, rowNumber)
		index := rowNumber / int64(shipModeTypeDist.Size())
		smCodeVal := valueForIndexModSize(shipModeCodeDist, index)
		smCarrierVal := shipModeCarrierDist.ValueAtIndex(0, int(rowNumber)-1)
		smContractVal := generateRandomCharset(alphaNumeric, 1, 20, ss.at(smContract))

		return []any{
			rowNumber,                  // sm_ship_mode_sk
			MakeBusinessKey(rowNumber), // sm_ship_mode_id
			smTypeVal,                  // sm_type
			smCodeVal,                  // sm_code
			smCarrierVal,               // sm_carrier
			smContractVal,              // sm_contract
		}
	},
}
