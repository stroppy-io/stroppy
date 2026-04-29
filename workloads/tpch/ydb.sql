-- TPC-H workload for YDB (YQL via the native driver). Schema follows the
-- TPC-H spec §1.4 shape with YQL type substitutions:
--   - CHAR(N) / VARCHAR(N) → Utf8 (YDB row tables have no fixed-width CHAR).
--   - Currency columns → Double. Framework emits float64 from Draw.decimal;
--     Expr.lit(0.0) needs litDouble() in tx.ts to keep zero-initialized
--     o_totalprice on the Double wire (see workloads/tpch/tx.ts).
--   - No FOREIGN KEY support; PRIMARY KEY only.
--   - DATE literals: `DATE '1998-12-01'` → `CAST('1998-12-01' AS Timestamp)`.
--
-- Secondary indexes are skipped — YDB row tables already shard on PRIMARY
-- KEY; secondary materialization has no query benefit for the full-scan
-- analytic shape of TPC-H.
--
-- Query rewrites vs pg.sql (permissible per spec §2.2.3.3):
--   - Comma-joins → CROSS JOIN (§2.2.3.3 (q)).
--   - Correlated subqueries decorrelated into named $subqueries (§(m)/(q)).
--     Affected queries: q2, q4, q15, q17, q20, q21, q22.
--   - extract(year FROM ...) → DateTime::GetYear(DateTime::Split(...)).
--   - substring(x FROM a FOR b) → Substring(CAST(x AS String), a-1, b).
--
-- Q15 lifts the spec CTE to a YQL named subquery `$revenue = (SELECT ...);`
-- Q17/Q20 precompute the spec per-part thresholds via JOIN-on-aggregate.
-- Q21 decorrelates the two correlated EXISTS subqueries into $multi and
-- $late_per_order sets.
-- Q22 rewrites the NOT EXISTS correlated subquery as NOT IN on orders.o_custkey.

--+ drop_schema
--= drop_lineitem
DROP TABLE IF EXISTS lineitem
--= drop_partsupp
DROP TABLE IF EXISTS partsupp
--= drop_orders
DROP TABLE IF EXISTS orders
--= drop_customer
DROP TABLE IF EXISTS customer
--= drop_supplier
DROP TABLE IF EXISTS supplier
--= drop_part
DROP TABLE IF EXISTS part
--= drop_nation
DROP TABLE IF EXISTS nation
--= drop_region
DROP TABLE IF EXISTS region

--+ create_schema
--= create_region
CREATE TABLE region (
    r_regionkey  Int64           NOT NULL,
    r_name       Utf8            NOT NULL,
    r_comment    Utf8,
    PRIMARY KEY (r_regionkey)
)
--= create_nation
CREATE TABLE nation (
    n_nationkey  Int64           NOT NULL,
    n_name       Utf8            NOT NULL,
    n_regionkey  Int64           NOT NULL,
    n_comment    Utf8,
    PRIMARY KEY (n_nationkey)
)
--= create_part
CREATE TABLE part (
    p_partkey     Int64           NOT NULL,
    p_name        Utf8            NOT NULL,
    p_mfgr        Utf8            NOT NULL,
    p_brand       Utf8            NOT NULL,
    p_type        Utf8            NOT NULL,
    p_size        Int64           NOT NULL,
    p_container   Utf8            NOT NULL,
    p_retailprice Double          NOT NULL,
    p_comment     Utf8            NOT NULL,
    PRIMARY KEY (p_partkey)
)
--= create_supplier
CREATE TABLE supplier (
    s_suppkey    Int64           NOT NULL,
    s_name       Utf8            NOT NULL,
    s_address    Utf8            NOT NULL,
    s_nationkey  Int64           NOT NULL,
    s_phone      Utf8            NOT NULL,
    s_acctbal    Double          NOT NULL,
    s_comment    Utf8            NOT NULL,
    PRIMARY KEY (s_suppkey)
)
--= create_partsupp
CREATE TABLE partsupp (
    ps_partkey    Int64          NOT NULL,
    ps_suppkey    Int64          NOT NULL,
    ps_availqty   Int64          NOT NULL,
    ps_supplycost Double         NOT NULL,
    ps_comment    Utf8           NOT NULL,
    PRIMARY KEY (ps_partkey, ps_suppkey)
)
--= create_customer
CREATE TABLE customer (
    c_custkey     Int64           NOT NULL,
    c_name        Utf8            NOT NULL,
    c_address     Utf8            NOT NULL,
    c_nationkey   Int64           NOT NULL,
    c_phone       Utf8            NOT NULL,
    c_acctbal     Double          NOT NULL,
    c_mktsegment  Utf8            NOT NULL,
    c_comment     Utf8            NOT NULL,
    PRIMARY KEY (c_custkey)
)
--= create_orders
CREATE TABLE orders (
    o_orderkey      Int64           NOT NULL,
    o_custkey       Int64           NOT NULL,
    o_orderstatus   Utf8            NOT NULL,
    o_totalprice    Double          NOT NULL,
    o_orderdate     Timestamp       NOT NULL,
    o_orderpriority Utf8            NOT NULL,
    o_clerk         Utf8            NOT NULL,
    o_shippriority  Int64           NOT NULL,
    o_comment       Utf8            NOT NULL,
    PRIMARY KEY (o_orderkey)
)
--= create_lineitem
CREATE TABLE lineitem (
    l_orderkey      Int64           NOT NULL,
    l_partkey       Int64           NOT NULL,
    l_suppkey       Int64           NOT NULL,
    l_linenumber    Int64           NOT NULL,
    l_quantity      Double          NOT NULL,
    l_extendedprice Double          NOT NULL,
    l_discount      Double          NOT NULL,
    l_tax           Double          NOT NULL,
    l_returnflag    Utf8            NOT NULL,
    l_linestatus    Utf8            NOT NULL,
    l_shipdate      Timestamp       NOT NULL,
    l_commitdate    Timestamp       NOT NULL,
    l_receiptdate   Timestamp       NOT NULL,
    l_shipinstruct  Utf8            NOT NULL,
    l_shipmode      Utf8            NOT NULL,
    l_comment       Utf8            NOT NULL,
    PRIMARY KEY (l_orderkey, l_linenumber)
)

--+ create_indexes
-- YDB row tables key-shard on PRIMARY KEY; secondary indexes carry
-- materialization cost without query benefit for full-scan analytics.
-- The spec lists indexes as auxiliary, not required.
--= noop
SELECT 1

--+ finalize_totals
-- Spec §4.2.3 o_totalprice = Σ l_extendedprice × (1 + l_tax) × (1 - l_discount).
-- YDB's UPDATE supports a correlated scalar subquery. Use UPSERT-style
-- SET with a CTE lifted into a named subquery so the planner can batch.
--= update_totalprice
$per_order = (
    SELECT l_orderkey,
           SUM(l_extendedprice * (1.0 + l_tax) * (1.0 - l_discount)) AS tot
    FROM   lineitem
    GROUP  BY l_orderkey
);
UPDATE orders ON
SELECT o.o_orderkey AS o_orderkey,
       o.o_custkey AS o_custkey,
       o.o_orderstatus AS o_orderstatus,
       COALESCE(p.tot, 0.0) AS o_totalprice,
       o.o_orderdate AS o_orderdate,
       o.o_orderpriority AS o_orderpriority,
       o.o_clerk AS o_clerk,
       o.o_shippriority AS o_shippriority,
       o.o_comment AS o_comment
FROM   orders AS o
       LEFT JOIN $per_order AS p ON p.l_orderkey = o.o_orderkey

-- ==========================================================================
-- 22 TPC-H queries, YQL port. Permissible deviations per §2.2.3.3.
-- ==========================================================================

--+ q1
--= body
SELECT l_returnflag, l_linestatus,
       sum(l_quantity) AS sum_qty,
       sum(l_extendedprice) AS sum_base_price,
       sum(l_extendedprice * (1.0 - l_discount)) AS sum_disc_price,
       sum(l_extendedprice * (1.0 - l_discount) * (1.0 + l_tax)) AS sum_charge,
       avg(l_quantity) AS avg_qty,
       avg(l_extendedprice) AS avg_price,
       avg(l_discount) AS avg_disc,
       count(*) AS count_order
FROM   lineitem
WHERE  l_shipdate <= CAST('1998-12-01' AS Timestamp) - DateTime::IntervalFromDays(CAST(:delta AS Int64))
GROUP  BY l_returnflag, l_linestatus
ORDER  BY l_returnflag, l_linestatus

--+ q2
--= body
-- Decorrelated: precompute min(ps_supplycost) per (partkey, region).
$min_cost = (
    SELECT ps2.ps_partkey AS partkey,
           min(ps2.ps_supplycost) AS mc
    FROM   partsupp AS ps2
           CROSS JOIN supplier AS s2
           CROSS JOIN nation AS n2
           CROSS JOIN region AS r2
    WHERE  s2.s_suppkey = ps2.ps_suppkey
      AND  s2.s_nationkey = n2.n_nationkey
      AND  n2.n_regionkey = r2.r_regionkey
      AND  r2.r_name = :region
    GROUP  BY ps2.ps_partkey
);
SELECT s.s_acctbal, s.s_name, n.n_name, p.p_partkey, p.p_mfgr,
       s.s_address, s.s_phone, s.s_comment
FROM   part AS p
       CROSS JOIN supplier AS s
       CROSS JOIN partsupp AS ps
       CROSS JOIN nation AS n
       CROSS JOIN region AS r
       CROSS JOIN $min_cost AS mc
WHERE  p.p_partkey = ps.ps_partkey
  AND  s.s_suppkey = ps.ps_suppkey
  AND  p.p_size   = :size
  AND  p.p_type LIKE '%' || :type
  AND  s.s_nationkey = n.n_nationkey
  AND  n.n_regionkey = r.r_regionkey
  AND  r.r_name = :region
  AND  mc.partkey = p.p_partkey
  AND  ps.ps_supplycost = mc.mc
ORDER BY s_acctbal DESC, n_name, s_name, p_partkey
LIMIT 100

--+ q3
--= body
SELECT l.l_orderkey AS l_orderkey,
       sum(l.l_extendedprice * (1.0 - l.l_discount)) AS revenue,
       o.o_orderdate AS o_orderdate,
       o.o_shippriority AS o_shippriority
FROM   customer AS c
       CROSS JOIN orders AS o
       CROSS JOIN lineitem AS l
WHERE  c.c_mktsegment = :segment
  AND  c.c_custkey = o.o_custkey
  AND  l.l_orderkey = o.o_orderkey
  AND  o.o_orderdate < CAST(:date AS Timestamp)
  AND  l.l_shipdate  > CAST(:date AS Timestamp)
GROUP  BY l.l_orderkey, o.o_orderdate, o.o_shippriority
ORDER  BY revenue DESC, o_orderdate
LIMIT 10

--+ q4
--= body
-- Correlated EXISTS decorrelated via IN-on-dedup-orderkeys.
SELECT o_orderpriority, count(*) AS order_count
FROM   orders
WHERE  o_orderdate >= CAST(:date AS Timestamp)
  AND  o_orderdate <  CAST(:date_3m AS Timestamp)
  AND  o_orderkey IN (
       SELECT DISTINCT l_orderkey FROM lineitem
       WHERE  l_commitdate < l_receiptdate
  )
GROUP  BY o_orderpriority
ORDER  BY o_orderpriority

--+ q5
--= body
SELECT n.n_name AS n_name,
       sum(l.l_extendedprice * (1.0 - l.l_discount)) AS revenue
FROM   customer AS c
       CROSS JOIN orders AS o
       CROSS JOIN lineitem AS l
       CROSS JOIN supplier AS s
       CROSS JOIN nation AS n
       CROSS JOIN region AS r
WHERE  c.c_custkey = o.o_custkey
  AND  l.l_orderkey = o.o_orderkey
  AND  l.l_suppkey = s.s_suppkey
  AND  c.c_nationkey = s.s_nationkey
  AND  s.s_nationkey = n.n_nationkey
  AND  n.n_regionkey = r.r_regionkey
  AND  r.r_name = :region
  AND  o.o_orderdate >= CAST(:date AS Timestamp)
  AND  o.o_orderdate <  CAST(:date_1y AS Timestamp)
GROUP  BY n.n_name
ORDER  BY revenue DESC

--+ q6
--= body
SELECT sum(l_extendedprice * l_discount) AS revenue
FROM   lineitem
WHERE  l_shipdate >= CAST(:date AS Timestamp)
  AND  l_shipdate <  CAST(:date_1y AS Timestamp)
  AND  l_discount BETWEEN :discount - 0.01 AND :discount + 0.01
  AND  l_quantity < :quantity

--+ q7
--= body
SELECT supp_nation, cust_nation, l_year, sum(volume) AS revenue
FROM (
  SELECT n1.n_name AS supp_nation,
         n2.n_name AS cust_nation,
         DateTime::GetYear(l_shipdate) AS l_year,
         l_extendedprice * (1.0 - l_discount) AS volume
  FROM   supplier
         CROSS JOIN lineitem
         CROSS JOIN orders
         CROSS JOIN customer
         CROSS JOIN nation AS n1
         CROSS JOIN nation AS n2
  WHERE  s_suppkey = l_suppkey
    AND  o_orderkey = l_orderkey
    AND  c_custkey = o_custkey
    AND  s_nationkey = n1.n_nationkey
    AND  c_nationkey = n2.n_nationkey
    AND  ( (n1.n_name = :nation1 AND n2.n_name = :nation2)
        OR (n1.n_name = :nation2 AND n2.n_name = :nation1))
    AND  l_shipdate BETWEEN CAST('1995-01-01' AS Timestamp) AND CAST('1996-12-31' AS Timestamp)
) AS shipping
GROUP  BY supp_nation, cust_nation, l_year
ORDER  BY supp_nation, cust_nation, l_year

--+ q8
--= body
SELECT o_year,
       sum(CASE WHEN nation = :nation THEN volume ELSE 0.0 END) / sum(volume) AS mkt_share
FROM (
  SELECT DateTime::GetYear(o_orderdate) AS o_year,
         l_extendedprice * (1.0 - l_discount) AS volume,
         n2.n_name AS nation
  FROM   part
         CROSS JOIN supplier
         CROSS JOIN lineitem
         CROSS JOIN orders
         CROSS JOIN customer
         CROSS JOIN nation AS n1
         CROSS JOIN nation AS n2
         CROSS JOIN region
  WHERE  p_partkey = l_partkey
    AND  s_suppkey = l_suppkey
    AND  l_orderkey = o_orderkey
    AND  o_custkey = c_custkey
    AND  c_nationkey = n1.n_nationkey
    AND  n1.n_regionkey = r_regionkey
    AND  r_name = :region
    AND  s_nationkey = n2.n_nationkey
    AND  o_orderdate BETWEEN CAST('1995-01-01' AS Timestamp) AND CAST('1996-12-31' AS Timestamp)
    AND  p_type = :type
) AS all_nations
GROUP  BY o_year
ORDER  BY o_year

--+ q9
--= body
SELECT nation, o_year, sum(amount) AS sum_profit
FROM (
  SELECT n_name AS nation,
         DateTime::GetYear(o_orderdate) AS o_year,
         l_extendedprice * (1.0 - l_discount) - ps_supplycost * l_quantity AS amount
  FROM   part
         CROSS JOIN supplier
         CROSS JOIN lineitem
         CROSS JOIN partsupp
         CROSS JOIN orders
         CROSS JOIN nation
  WHERE  s_suppkey = l_suppkey
    AND  ps_suppkey = l_suppkey
    AND  ps_partkey = l_partkey
    AND  p_partkey = l_partkey
    AND  o_orderkey = l_orderkey
    AND  s_nationkey = n_nationkey
    AND  p_name LIKE '%' || :color || '%'
) AS profit
GROUP  BY nation, o_year
ORDER  BY nation, o_year DESC

--+ q10
--= body
SELECT c.c_custkey AS c_custkey, c.c_name AS c_name,
       sum(l.l_extendedprice * (1.0 - l.l_discount)) AS revenue,
       c.c_acctbal AS c_acctbal, n.n_name AS n_name,
       c.c_address AS c_address, c.c_phone AS c_phone, c.c_comment AS c_comment
FROM   customer AS c
       CROSS JOIN orders AS o
       CROSS JOIN lineitem AS l
       CROSS JOIN nation AS n
WHERE  c.c_custkey = o.o_custkey
  AND  l.l_orderkey = o.o_orderkey
  AND  o.o_orderdate >= CAST(:date AS Timestamp)
  AND  o.o_orderdate <  CAST(:date_3m AS Timestamp)
  AND  l.l_returnflag = 'R'
  AND  c.c_nationkey = n.n_nationkey
GROUP  BY c.c_custkey, c.c_name, c.c_acctbal, c.c_phone, n.n_name, c.c_address, c.c_comment
ORDER  BY revenue DESC
LIMIT 20

--+ q11
--= body
SELECT ps.ps_partkey AS ps_partkey,
       sum(ps.ps_supplycost * ps.ps_availqty) AS value
FROM   partsupp AS ps
       CROSS JOIN supplier AS s
       CROSS JOIN nation AS n
WHERE  ps.ps_suppkey = s.s_suppkey
  AND  s.s_nationkey = n.n_nationkey
  AND  n.n_name = :nation
GROUP  BY ps.ps_partkey
HAVING sum(ps.ps_supplycost * ps.ps_availqty) > (
       SELECT sum(ps2.ps_supplycost * ps2.ps_availqty) * :fraction
       FROM   partsupp AS ps2
              CROSS JOIN supplier AS s2
              CROSS JOIN nation AS n2
       WHERE  ps2.ps_suppkey = s2.s_suppkey
         AND  s2.s_nationkey = n2.n_nationkey
         AND  n2.n_name = :nation
)
ORDER  BY value DESC

--+ q12
--= body
SELECT l.l_shipmode AS l_shipmode,
       sum(CASE WHEN o.o_orderpriority = '1-URGENT'
                 OR o.o_orderpriority = '2-HIGH'
                THEN 1 ELSE 0 END) AS high_line_count,
       sum(CASE WHEN o.o_orderpriority <> '1-URGENT'
                AND o.o_orderpriority <> '2-HIGH'
                THEN 1 ELSE 0 END) AS low_line_count
FROM   orders AS o
       CROSS JOIN lineitem AS l
WHERE  o.o_orderkey = l.l_orderkey
  AND  l.l_shipmode IN (:shipmode1, :shipmode2)
  AND  l.l_commitdate < l.l_receiptdate
  AND  l.l_shipdate   < l.l_commitdate
  AND  l.l_receiptdate >= CAST(:date AS Timestamp)
  AND  l.l_receiptdate <  CAST(:date_1y AS Timestamp)
GROUP  BY l.l_shipmode
ORDER  BY l_shipmode

--+ q13
--= body
-- YDB: JOIN ON must be a conjunction of equalities; the non-equi
-- NOT LIKE predicate moves into a derived table on the right-hand side.
SELECT c_count, count(*) AS custdist
FROM (
  SELECT c.c_custkey AS c_custkey, count(o.o_orderkey) AS c_count
  FROM   customer AS c
         LEFT JOIN (
            SELECT o2.o_custkey AS o_custkey, o2.o_orderkey AS o_orderkey
            FROM   orders AS o2
            WHERE  o2.o_comment NOT LIKE '%' || :word1 || '%' || :word2 || '%'
         ) AS o
                ON c.c_custkey = o.o_custkey
  GROUP  BY c.c_custkey
) AS c_orders
GROUP  BY c_count
ORDER  BY custdist DESC, c_count DESC

--+ q14
--= body
SELECT 100.0 *
       sum(CASE WHEN p.p_type LIKE 'PROMO%'
                THEN l.l_extendedprice * (1.0 - l.l_discount)
                ELSE 0.0 END)
       / sum(l.l_extendedprice * (1.0 - l.l_discount))
       AS promo_revenue
FROM   lineitem AS l
       CROSS JOIN part AS p
WHERE  l.l_partkey = p.p_partkey
  AND  l.l_shipdate >= CAST(:date AS Timestamp)
  AND  l.l_shipdate <  CAST(:date_1m AS Timestamp)

--+ q15
--= body
-- Spec CTE lifted to a YQL named subquery.
$revenue = (
    SELECT l_suppkey AS supplier_no,
           sum(l_extendedprice * (1.0 - l_discount)) AS total_revenue
    FROM   lineitem
    WHERE  l_shipdate >= CAST(:date AS Timestamp)
      AND  l_shipdate <  CAST(:date_3m AS Timestamp)
    GROUP  BY l_suppkey
);
SELECT s_suppkey, s_name, s_address, s_phone, total_revenue
FROM   supplier
       CROSS JOIN $revenue AS revenue
WHERE  s_suppkey = revenue.supplier_no
  AND  revenue.total_revenue = (SELECT max(total_revenue) FROM $revenue)
ORDER  BY s_suppkey

--+ q16
--= body
SELECT p.p_brand AS p_brand, p.p_type AS p_type, p.p_size AS p_size,
       count(DISTINCT ps.ps_suppkey) AS supplier_cnt
FROM   partsupp AS ps
       CROSS JOIN part AS p
WHERE  p.p_partkey = ps.ps_partkey
  AND  p.p_brand <> :brand
  AND  p.p_type NOT LIKE :type_prefix || '%'
  AND  p.p_size IN (:s1, :s2, :s3, :s4, :s5, :s6, :s7, :s8)
  AND  ps.ps_suppkey NOT IN (
       SELECT s.s_suppkey FROM supplier AS s
       WHERE  s.s_comment LIKE '%Customer%Complaints%'
  )
GROUP  BY p.p_brand, p.p_type, p.p_size
ORDER  BY supplier_cnt DESC, p_brand, p_type, p_size

--+ q17
--= body
-- Correlated on p_partkey → JOIN-on-aggregate per partkey.
$avg_qty = (
    SELECT l2.l_partkey AS partkey,
           0.2 * avg(l2.l_quantity) AS threshold
    FROM   lineitem AS l2
    GROUP  BY l2.l_partkey
);
SELECT sum(l.l_extendedprice) / 7.0 AS avg_yearly
FROM   lineitem AS l
       CROSS JOIN part AS p
       CROSS JOIN $avg_qty AS aq
WHERE  p.p_partkey = l.l_partkey
  AND  aq.partkey  = l.l_partkey
  AND  p.p_brand = :brand
  AND  p.p_container = :container
  AND  l.l_quantity < aq.threshold

--+ q18
--= body
SELECT c.c_name AS c_name, c.c_custkey AS c_custkey,
       o.o_orderkey AS o_orderkey, o.o_orderdate AS o_orderdate,
       o.o_totalprice AS o_totalprice, sum(l.l_quantity) AS sum_qty
FROM   customer AS c
       CROSS JOIN orders AS o
       CROSS JOIN lineitem AS l
WHERE  o.o_orderkey IN (
       SELECT l2.l_orderkey FROM lineitem AS l2
       GROUP  BY l2.l_orderkey
       HAVING sum(l2.l_quantity) > :quantity
  )
  AND  c.c_custkey = o.o_custkey
  AND  o.o_orderkey = l.l_orderkey
GROUP  BY c.c_name, c.c_custkey, o.o_orderkey, o.o_orderdate, o.o_totalprice
ORDER  BY o_totalprice DESC, o_orderdate
LIMIT 100

--+ q19
--= body
SELECT sum(l_extendedprice * (1.0 - l_discount)) AS revenue
FROM   lineitem
       CROSS JOIN part
WHERE  (
       p_partkey = l_partkey
  AND  p_brand = :brand1
  AND  p_container IN ('SM CASE', 'SM BOX', 'SM PACK', 'SM PKG')
  AND  l_quantity >= :q1 AND l_quantity <= :q1 + 10
  AND  p_size BETWEEN 1 AND 5
  AND  l_shipmode IN ('AIR', 'AIR REG')
  AND  l_shipinstruct = 'DELIVER IN PERSON'
)
OR (
       p_partkey = l_partkey
  AND  p_brand = :brand2
  AND  p_container IN ('MED BAG', 'MED BOX', 'MED PKG', 'MED PACK')
  AND  l_quantity >= :q2 AND l_quantity <= :q2 + 10
  AND  p_size BETWEEN 1 AND 10
  AND  l_shipmode IN ('AIR', 'AIR REG')
  AND  l_shipinstruct = 'DELIVER IN PERSON'
)
OR (
       p_partkey = l_partkey
  AND  p_brand = :brand3
  AND  p_container IN ('LG CASE', 'LG BOX', 'LG PACK', 'LG PKG')
  AND  l_quantity >= :q3 AND l_quantity <= :q3 + 10
  AND  p_size BETWEEN 1 AND 15
  AND  l_shipmode IN ('AIR', 'AIR REG')
  AND  l_shipinstruct = 'DELIVER IN PERSON'
)

--+ q20
--= body
-- Correlated on (ps_partkey, ps_suppkey) decorrelated via JOIN-on-aggregate.
$qty_window = (
    SELECT l.l_partkey AS partkey,
           l.l_suppkey AS suppkey,
           0.5 * sum(l.l_quantity) AS threshold
    FROM   lineitem AS l
    WHERE  l.l_shipdate >= CAST(:date AS Timestamp)
      AND  l.l_shipdate <  CAST(:date_1y AS Timestamp)
    GROUP  BY l.l_partkey, l.l_suppkey
);
SELECT s.s_name AS s_name, s.s_address AS s_address
FROM   supplier AS s
       CROSS JOIN nation AS n
WHERE  s.s_suppkey IN (
       SELECT ps.ps_suppkey
       FROM   partsupp AS ps
              CROSS JOIN $qty_window AS qw
       WHERE  ps.ps_partkey = qw.partkey
         AND  ps.ps_suppkey = qw.suppkey
         AND  CAST(ps.ps_availqty AS Double) > qw.threshold
         AND  ps.ps_partkey IN (
              SELECT p.p_partkey
              FROM   part AS p
              WHERE  p.p_name LIKE :color || '%'
         )
)
  AND  s.s_nationkey = n.n_nationkey
  AND  n.n_name = :nation
ORDER  BY s_name

--+ q21
--= body
-- Two correlated subqueries → $multi (orderkeys with >=2 distinct suppliers)
-- and $late_per_order (orderkey → distinct late-supplier count). Spec
-- "this supplier late, no other supplier late" = late_suppliers = 1.
$multi = (
    SELECT l_orderkey
    FROM   lineitem
    GROUP  BY l_orderkey
    HAVING count(DISTINCT l_suppkey) > 1
);
$late_per_order = (
    SELECT l_orderkey, count(DISTINCT l_suppkey) AS late_suppliers
    FROM   lineitem
    WHERE  l_receiptdate > l_commitdate
    GROUP  BY l_orderkey
);
SELECT s.s_name AS s_name, count(*) AS numwait
FROM   supplier AS s
       CROSS JOIN lineitem AS l1
       CROSS JOIN orders AS o
       CROSS JOIN nation AS n
       CROSS JOIN $multi AS m
       CROSS JOIN $late_per_order AS lp
WHERE  s.s_suppkey = l1.l_suppkey
  AND  o.o_orderkey = l1.l_orderkey
  AND  m.l_orderkey = l1.l_orderkey
  AND  lp.l_orderkey = l1.l_orderkey
  AND  lp.late_suppliers = 1
  AND  o.o_orderstatus = 'F'
  AND  l1.l_receiptdate > l1.l_commitdate
  AND  s.s_nationkey = n.n_nationkey
  AND  n.n_name = :nation
GROUP  BY s.s_name
ORDER  BY numwait DESC, s_name
LIMIT 100

--+ q22
--= body
-- NOT EXISTS correlated subquery rewritten as NOT IN on orders.o_custkey.
-- substring(phone FROM 1 FOR 2) → Substring(CAST(phone AS String), 0, 2).
SELECT cntrycode, count(*) AS numcust, sum(c_acctbal) AS totacctbal
FROM (
  SELECT Substring(CAST(c.c_phone AS String), 0u, 2u) AS cntrycode,
         c.c_acctbal AS c_acctbal
  FROM   customer AS c
  WHERE  Substring(CAST(c.c_phone AS String), 0u, 2u) IN
         (CAST(:cc1 AS String), CAST(:cc2 AS String), CAST(:cc3 AS String),
          CAST(:cc4 AS String), CAST(:cc5 AS String), CAST(:cc6 AS String),
          CAST(:cc7 AS String))
    AND  c.c_acctbal > (
         SELECT avg(c2.c_acctbal)
         FROM   customer AS c2
         WHERE  c2.c_acctbal > 0.0
           AND  Substring(CAST(c2.c_phone AS String), 0u, 2u) IN
                (CAST(:cc1 AS String), CAST(:cc2 AS String), CAST(:cc3 AS String),
                 CAST(:cc4 AS String), CAST(:cc5 AS String), CAST(:cc6 AS String),
                 CAST(:cc7 AS String))
    )
    AND  c.c_custkey NOT IN (SELECT o.o_custkey FROM orders AS o)
) AS custsale
GROUP  BY cntrycode
ORDER  BY cntrycode
