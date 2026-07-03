-- Supplementary queries for the stroppy-next tpcc port. pg.sql is embedded
-- verbatim from the v5 corpus; this file adds only what the client-side Tx port
-- needs that the v5 workload_tx_* sections express with client-side IN-list
-- string interpolation — which the prepared-handle Args API deliberately does
-- not support (no array bind, no per-call SQL rewrite). It is folded into a
-- single prepared statement here.

--+ workload_tx_stock_level
--= count_low_stock
-- One-shot stock_level count: the last-20-orders item window joined to stock,
-- counting distinct low-stock items. Equivalent to v5's two-step
-- get_window_items + stock_count_in (and to the SLEV proc body), collapsed to a
-- single prepared statement so no IN-list is built or interpolated on the hot
-- path.
SELECT COUNT(DISTINCT s.s_i_id)
FROM order_line ol
JOIN stock s ON s.s_w_id = ol.ol_w_id AND s.s_i_id = ol.ol_i_id
WHERE ol.ol_w_id = :w_id
  AND ol.ol_d_id = :d_id
  AND ol.ol_o_id >= :min_o_id
  AND ol.ol_o_id < :next_o_id
  AND s.s_quantity < :threshold
