-- TPC-H workload for picodata (Tarantool / sbroad). Schema follows TPC-H
-- spec §1.4; sbroad's SQL subset is pg-like with the following caveats
-- which shape this file:
--   - CHAR(N) → VARCHAR(N) (sbroad lacks fixed-width CHAR).
--   - DATE → DATETIME (sbroad stores dates as Tarantool datetime).
--   - No FK constraints; only PRIMARY KEY.
--
-- sbroad planner/grammar gaps that forced rewrites vs the pg text (every
-- rewrite below is answer-checked against postgres on identical data at
-- SF=0.01 — result sets are row-for-row equal):
--   - Implicit comma joins (FROM a, b) are rejected → explicit JOIN ON.
--   - Typed date literal `date '1998-12-01'` is rejected → bare string
--     '1998-12-01' (DATETIME columns compare fine against a string).
--   - `interval` arithmetic is unsupported → q1's `date - interval` cutoff
--     is precomputed client-side and bound as :shipdate_cutoff; the other
--     window ends are shifted client-side and bound as :date_1m/_3m/_1y
--     (tx.ts NEEDS_END_DATES).
--   - `extract(year FROM col)` is rejected → substring(cast(col AS string)
--     FROM 1 FOR 4) in q7/q8/q9.
--   - `NOT LIKE` is rejected (even `x NOT LIKE 'literal'`) → NOT (x LIKE …)
--     in q13/q16.
--   - Correlated outer-column refs inside a subquery are unresolvable
--     ("column … and scan Some(…) not found"). q2/q17/q20/q21 are
--     decorrelated via JOIN-on-aggregate CTEs; q4/q22 swap correlated
--     EXISTS/NOT EXISTS for uncorrelated IN / NOT IN.
-- The post-load totalprice recompute (UPDATE-with-correlated-subquery) has
-- no sbroad equivalent, but the datagen runtime emits the real o_totalprice
-- per spec §4.2.3 at load time, so q18 sees correct values without it.
--
-- sbroad resource caps: the multi-join aggregates (q3, q10, q21) fan out
-- large intermediate rows and need sql_vdbe_opcode_max and
-- sql_motion_row_max raised well above sbroad defaults; the tmpfs-all
-- compose init does both before this workload runs.

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
    r_regionkey  INTEGER        NOT NULL,
    r_name       VARCHAR(25)    NOT NULL,
    r_comment    VARCHAR(152),
    PRIMARY KEY (r_regionkey)
)
--= create_nation
CREATE TABLE nation (
    n_nationkey  INTEGER        NOT NULL,
    n_name       VARCHAR(25)    NOT NULL,
    n_regionkey  INTEGER        NOT NULL,
    n_comment    VARCHAR(152),
    PRIMARY KEY (n_nationkey)
)
--= create_part
CREATE TABLE part (
    p_partkey     BIGINT         NOT NULL,
    p_name        VARCHAR(55)    NOT NULL,
    p_mfgr        VARCHAR(25)    NOT NULL,
    p_brand       VARCHAR(10)    NOT NULL,
    p_type        VARCHAR(25)    NOT NULL,
    p_size        INTEGER        NOT NULL,
    p_container   VARCHAR(10)    NOT NULL,
    p_retailprice DECIMAL(12,2)  NOT NULL,
    p_comment     VARCHAR(23)    NOT NULL,
    PRIMARY KEY (p_partkey)
)
--= create_supplier
CREATE TABLE supplier (
    s_suppkey    INTEGER         NOT NULL,
    s_name       VARCHAR(25)     NOT NULL,
    s_address    VARCHAR(40)     NOT NULL,
    s_nationkey  INTEGER         NOT NULL,
    s_phone      VARCHAR(15)     NOT NULL,
    s_acctbal    DECIMAL(12,2)   NOT NULL,
    s_comment    VARCHAR(101)    NOT NULL,
    PRIMARY KEY (s_suppkey)
)
--= create_partsupp
CREATE TABLE partsupp (
    ps_partkey    BIGINT         NOT NULL,
    ps_suppkey    INTEGER        NOT NULL,
    ps_availqty   INTEGER        NOT NULL,
    ps_supplycost DECIMAL(12,2)  NOT NULL,
    ps_comment    VARCHAR(199)   NOT NULL,
    PRIMARY KEY (ps_partkey, ps_suppkey)
)
--= create_customer
CREATE TABLE customer (
    c_custkey     INTEGER         NOT NULL,
    c_name        VARCHAR(25)     NOT NULL,
    c_address     VARCHAR(40)     NOT NULL,
    c_nationkey   INTEGER         NOT NULL,
    c_phone       VARCHAR(15)     NOT NULL,
    c_acctbal     DECIMAL(12,2)   NOT NULL,
    c_mktsegment  VARCHAR(10)     NOT NULL,
    c_comment     VARCHAR(117)    NOT NULL,
    PRIMARY KEY (c_custkey)
)
--= create_orders
CREATE TABLE orders (
    o_orderkey      BIGINT          NOT NULL,
    o_custkey       INTEGER         NOT NULL,
    o_orderstatus   VARCHAR(1)      NOT NULL,
    o_totalprice    DECIMAL(12,2)   NOT NULL,
    o_orderdate     DATETIME        NOT NULL,
    o_orderpriority VARCHAR(15)     NOT NULL,
    o_clerk         VARCHAR(15)     NOT NULL,
    o_shippriority  INTEGER         NOT NULL,
    o_comment       VARCHAR(79)     NOT NULL,
    PRIMARY KEY (o_orderkey)
)
--= create_lineitem
CREATE TABLE lineitem (
    l_orderkey      BIGINT          NOT NULL,
    l_partkey       BIGINT          NOT NULL,
    l_suppkey       INTEGER         NOT NULL,
    l_linenumber    INTEGER         NOT NULL,
    l_quantity      DECIMAL(12,2)   NOT NULL,
    l_extendedprice DECIMAL(12,2)   NOT NULL,
    l_discount      DECIMAL(12,2)   NOT NULL,
    l_tax           DECIMAL(12,2)   NOT NULL,
    l_returnflag    VARCHAR(1)      NOT NULL,
    l_linestatus    VARCHAR(1)      NOT NULL,
    l_shipdate      DATETIME        NOT NULL,
    l_commitdate    DATETIME        NOT NULL,
    l_receiptdate   DATETIME        NOT NULL,
    l_shipinstruct  VARCHAR(25)     NOT NULL,
    l_shipmode      VARCHAR(10)     NOT NULL,
    l_comment       VARCHAR(44)     NOT NULL,
    PRIMARY KEY (l_orderkey, l_linenumber)
)

--+ create_indexes
--= idx_supplier_nationkey
CREATE INDEX idx_supplier_nationkey ON supplier (s_nationkey)
--= idx_partsupp_partkey
CREATE INDEX idx_partsupp_partkey    ON partsupp (ps_partkey)
--= idx_partsupp_suppkey
CREATE INDEX idx_partsupp_suppkey    ON partsupp (ps_suppkey)
--= idx_customer_nationkey
CREATE INDEX idx_customer_nationkey  ON customer (c_nationkey)
--= idx_orders_custkey
CREATE INDEX idx_orders_custkey      ON orders   (o_custkey)
--= idx_lineitem_partkey
CREATE INDEX idx_lineitem_partkey    ON lineitem (l_partkey)
--= idx_lineitem_suppkey
CREATE INDEX idx_lineitem_suppkey    ON lineitem (l_suppkey)
--= idx_lineitem_orderkey
CREATE INDEX idx_lineitem_orderkey   ON lineitem (l_orderkey)
--= idx_nation_regionkey
CREATE INDEX idx_nation_regionkey    ON nation   (n_regionkey)
--= idx_lineitem_shipdate
CREATE INDEX idx_lineitem_shipdate   ON lineitem (l_shipdate)
--= idx_orders_orderdate
CREATE INDEX idx_orders_orderdate    ON orders   (o_orderdate)

--+ finalize_totals
-- The spec §4.2.3 recompute is an UPDATE-with-correlated-subquery, which
-- sbroad cannot plan. It is unnecessary here: the datagen runtime emits the
-- real o_totalprice (Σ l_extendedprice × (1+l_tax) × (1-l_discount)) at
-- orders-emit time, so q18 sees correct values without a post-load UPDATE.
-- Placeholder step body kept so `--steps finalize_totals` runs cleanly.
--= noop
SELECT 1 FROM region WHERE r_regionkey = -1

-- ==========================================================================
-- 22 TPC-H queries, picodata port. Parameters follow §2.4.x defaults.
-- ==========================================================================

--+ q1
--= body
SELECT l_returnflag, l_linestatus,
       sum(l_quantity) AS sum_qty,
       sum(l_extendedprice) AS sum_base_price,
       sum(l_extendedprice * (1 - l_discount)) AS sum_disc_price,
       sum(l_extendedprice * (1 - l_discount) * (1 + l_tax)) AS sum_charge,
       avg(l_quantity) AS avg_qty,
       avg(l_extendedprice) AS avg_price,
       avg(l_discount) AS avg_disc,
       count(*) AS count_order
FROM   lineitem
WHERE  l_shipdate <= :shipdate_cutoff
GROUP  BY l_returnflag, l_linestatus
ORDER  BY l_returnflag, l_linestatus

--+ q2
--= body
-- Decorrelated: precompute min(ps_supplycost) per partkey in :region.
WITH mincost(p_partkey, mincost) AS (
    SELECT ps_partkey, min(ps_supplycost)
    FROM   partsupp ps
           JOIN supplier s   ON s.s_suppkey = ps.ps_suppkey
           JOIN nation n     ON s.s_nationkey = n.n_nationkey
           JOIN region r     ON n.n_regionkey = r.r_regionkey
    WHERE  r.r_name = :region
    GROUP  BY ps.ps_partkey
)
SELECT s.s_acctbal, s.s_name, n.n_name, p.p_partkey, p.p_mfgr,
       s.s_address, s.s_phone, s.s_comment
FROM   part p
       JOIN partsupp ps ON p.p_partkey = ps.ps_partkey
       JOIN supplier s  ON s.s_suppkey = ps.ps_suppkey
       JOIN nation n    ON s.s_nationkey = n.n_nationkey
       JOIN region r    ON n.n_regionkey = r.r_regionkey
       JOIN mincost m   ON m.p_partkey = p.p_partkey
WHERE  p.p_size = :size
  AND  p.p_type LIKE '%' || :type
  AND  r.r_name = :region
  AND  ps.ps_supplycost = m.mincost
ORDER BY s.s_acctbal DESC, n.n_name, s.s_name, p.p_partkey
LIMIT 100

--+ q3
--= body
SELECT l_orderkey,
       sum(l_extendedprice * (1 - l_discount)) AS revenue,
       o.o_orderdate,
       o_shippriority
FROM   customer c
       JOIN orders o   ON c.c_custkey = o.o_custkey
       JOIN lineitem l ON l.l_orderkey = o.o_orderkey
WHERE  c.c_mktsegment = :segment
  AND  o.o_orderdate < :date
  AND  l.l_shipdate  > :date
GROUP  BY l_orderkey, o.o_orderdate, o_shippriority
ORDER  BY revenue DESC, o.o_orderdate
LIMIT 10

--+ q4
--= body
-- Correlated EXISTS decorrelated via IN on the qualifying orderkeys.
SELECT o_orderpriority, count(*) AS order_count
FROM   orders
WHERE  o_orderdate >= :date
  AND  o_orderdate <  :date_3m
  AND  o_orderkey IN (
       SELECT l_orderkey FROM lineitem WHERE l_commitdate < l_receiptdate
  )
GROUP  BY o_orderpriority
ORDER  BY o_orderpriority

--+ q5
--= body
SELECT n_name, sum(l_extendedprice * (1 - l_discount)) AS revenue
FROM   customer c
       JOIN orders o   ON c.c_custkey = o.o_custkey
       JOIN lineitem l ON l.l_orderkey = o.o_orderkey
       JOIN supplier s ON l.l_suppkey = s.s_suppkey
       JOIN nation n   ON s.s_nationkey = n.n_nationkey
       JOIN region r   ON n.n_regionkey = r.r_regionkey
WHERE  c.c_nationkey = s.s_nationkey
  AND  r.r_name = :region
  AND  o.o_orderdate >= :date
  AND  o.o_orderdate <  :date_1y
GROUP  BY n_name
ORDER  BY revenue DESC

--+ q6
--= body
SELECT sum(l_extendedprice * l_discount) AS revenue
FROM   lineitem
WHERE  l_shipdate >= :date
  AND  l_shipdate <  :date_1y
  AND  l_discount BETWEEN :discount - 0.01 AND :discount + 0.01
  AND  l_quantity < :quantity

--+ q7
--= body
SELECT supp_nation, cust_nation, l_year, sum(volume) AS revenue
FROM (
  SELECT n1.n_name AS supp_nation,
         n2.n_name AS cust_nation,
         substring(cast(l_shipdate AS string) FROM 1 FOR 4) AS l_year,
         l_extendedprice * (1 - l_discount) AS volume
  FROM   supplier s
         JOIN lineitem l ON s.s_suppkey = l.l_suppkey
         JOIN orders o   ON o.o_orderkey = l.l_orderkey
         JOIN customer c ON c.c_custkey = o.o_custkey
         JOIN nation n1  ON s.s_nationkey = n1.n_nationkey
         JOIN nation n2  ON c.c_nationkey = n2.n_nationkey
  WHERE  ( (n1.n_name = :nation1 AND n2.n_name = :nation2)
        OR (n1.n_name = :nation2 AND n2.n_name = :nation1))
    AND  l_shipdate BETWEEN '1995-01-01' AND '1996-12-31'
) AS shipping
GROUP  BY supp_nation, cust_nation, l_year
ORDER  BY supp_nation, cust_nation, l_year

--+ q8
--= body
SELECT o_year,
       sum(CASE WHEN nation = :nation THEN volume ELSE 0 END) / sum(volume) AS mkt_share
FROM (
  SELECT substring(cast(o_orderdate AS string) FROM 1 FOR 4) AS o_year,
         l_extendedprice * (1 - l_discount) AS volume,
         n2.n_name AS nation
  FROM   part p
         JOIN lineitem l ON p.p_partkey = l.l_partkey
         JOIN supplier s ON s.s_suppkey = l.l_suppkey
         JOIN orders o   ON l.l_orderkey = o.o_orderkey
         JOIN customer c ON o.o_custkey = c.c_custkey
         JOIN nation n1  ON c.c_nationkey = n1.n_nationkey
         JOIN region r   ON n1.n_regionkey = r.r_regionkey
         JOIN nation n2  ON s.s_nationkey = n2.n_nationkey
  WHERE  r.r_name = :region
    AND  o_orderdate BETWEEN '1995-01-01' AND '1996-12-31'
    AND  p_type = :type
) AS all_nations
GROUP  BY o_year
ORDER  BY o_year

--+ q9
--= body
SELECT nation, o_year, sum(amount) AS sum_profit
FROM (
  SELECT n.n_name AS nation,
         substring(cast(o_orderdate AS string) FROM 1 FOR 4) AS o_year,
         l_extendedprice * (1 - l_discount) - ps_supplycost * l_quantity AS amount
  FROM   part p
         JOIN lineitem l ON p.p_partkey = l.l_partkey
         JOIN supplier s ON s.s_suppkey = l.l_suppkey
         JOIN partsupp ps ON ps.ps_suppkey = l.l_suppkey AND ps.ps_partkey = l.l_partkey
         JOIN orders o   ON o.o_orderkey = l.l_orderkey
         JOIN nation n   ON s.s_nationkey = n.n_nationkey
  WHERE  p_name LIKE '%' || :color || '%'
) AS profit
GROUP  BY nation, o_year
ORDER  BY nation, o_year DESC

--+ q10
--= body
SELECT c_custkey, c_name,
       sum(l_extendedprice * (1 - l_discount)) AS revenue,
       c_acctbal, n_name, c_address, c_phone, c_comment
FROM   customer c
       JOIN orders o   ON c.c_custkey = o.o_custkey
       JOIN lineitem l ON l.l_orderkey = o.o_orderkey
       JOIN nation n   ON c.c_nationkey = n.n_nationkey
WHERE  o.o_orderdate >= :date
  AND  o.o_orderdate <  :date_3m
  AND  l_returnflag = 'R'
GROUP  BY c_custkey, c_name, c_acctbal, c_phone, n_name, c_address, c_comment
ORDER  BY revenue DESC
LIMIT 20

--+ q11
--= body
SELECT ps_partkey, sum(ps_supplycost * ps_availqty) AS value
FROM   partsupp ps
       JOIN supplier s ON ps.ps_suppkey = s.s_suppkey
       JOIN nation n   ON s.s_nationkey = n.n_nationkey
WHERE  n.n_name = :nation
GROUP  BY ps_partkey
HAVING sum(ps_supplycost * ps_availqty) > (
       SELECT sum(ps2.ps_supplycost * ps2.ps_availqty) * cast(:fraction AS decimal)
       FROM   partsupp ps2
              JOIN supplier s2 ON ps2.ps_suppkey = s2.s_suppkey
              JOIN nation n2   ON s2.s_nationkey = n2.n_nationkey
       WHERE  n2.n_name = :nation
)
ORDER  BY value DESC

--+ q12
--= body
SELECT l_shipmode,
       sum(CASE WHEN o_orderpriority = '1-URGENT'
                 OR o_orderpriority = '2-HIGH'
                THEN 1 ELSE 0 END) AS high_line_count,
       sum(CASE WHEN o_orderpriority <> '1-URGENT'
                AND o_orderpriority <> '2-HIGH'
                THEN 1 ELSE 0 END) AS low_line_count
FROM   orders o
       JOIN lineitem l ON o.o_orderkey = l.l_orderkey
WHERE  l.l_shipmode IN (:shipmode1, :shipmode2)
  AND  l.l_commitdate < l.l_receiptdate
  AND  l.l_shipdate   < l.l_commitdate
  AND  l.l_receiptdate >= :date
  AND  l.l_receiptdate <  :date_1y
GROUP  BY l_shipmode
ORDER  BY l_shipmode

--+ q13
--= body
-- sbroad rejects NOT LIKE; the join-predicate negation moves inside NOT(…).
SELECT c_count, count(*) AS custdist
FROM (
  SELECT c.c_custkey, count(o.o_orderkey) AS c_count
  FROM   customer c
         LEFT JOIN orders o ON c.c_custkey = o.o_custkey
                           AND NOT (o.o_comment LIKE '%' || :word1 || '%' || :word2 || '%')
  GROUP  BY c.c_custkey
) AS c_orders
GROUP  BY c_count
ORDER  BY custdist DESC, c_count DESC

--+ q14
--= body
SELECT 100.00 * sum(CASE WHEN p_type LIKE 'PROMO%'
                         THEN l_extendedprice * (1 - l_discount)
                         ELSE 0 END)
               / sum(l_extendedprice * (1 - l_discount)) AS promo_revenue
FROM   lineitem l
       JOIN part p ON l.l_partkey = p.p_partkey
WHERE  l.l_shipdate >= :date
  AND  l.l_shipdate <  :date_1m

--+ q15
--= body
WITH revenue(supplier_no, total_revenue) AS (
    SELECT l_suppkey, sum(l_extendedprice * (1 - l_discount))
    FROM   lineitem
    WHERE  l_shipdate >= :date
      AND  l_shipdate <  :date_3m
    GROUP  BY l_suppkey
)
SELECT s_suppkey, s_name, s_address, s_phone, total_revenue
FROM   supplier s
       JOIN revenue ON s.s_suppkey = supplier_no
WHERE  total_revenue = (SELECT max(total_revenue) FROM revenue)
ORDER  BY s_suppkey

--+ q16
--= body
SELECT p_brand, p_type, p_size, count(DISTINCT ps_suppkey) AS supplier_cnt
FROM   partsupp ps
       JOIN part p ON p.p_partkey = ps.ps_partkey
WHERE  p.p_brand <> :brand
  AND  NOT (p.p_type LIKE :type_prefix || '%')
  AND  p.p_size IN (:s1, :s2, :s3, :s4, :s5, :s6, :s7, :s8)
  AND  ps.ps_suppkey NOT IN (
       SELECT s_suppkey FROM supplier
       WHERE  s_comment LIKE '%Customer%Complaints%'
  )
GROUP  BY p_brand, p_type, p_size
ORDER  BY supplier_cnt DESC, p_brand, p_type, p_size

--+ q17
--= body
-- Correlated on p_partkey → JOIN-on-aggregate per partkey.
WITH partavg(l_partkey, avgq) AS (
    SELECT l_partkey, 0.2 * avg(l_quantity) FROM lineitem GROUP BY l_partkey
)
SELECT sum(l_extendedprice) / 7.0 AS avg_yearly
FROM   lineitem l
       JOIN part p    ON p.p_partkey = l.l_partkey
       JOIN partavg pa ON pa.l_partkey = l.l_partkey
WHERE  p.p_brand = :brand
  AND  p.p_container = :container
  AND  l.l_quantity < pa.avgq

--+ q18
--= body
SELECT c_name, c_custkey, o_orderkey, o_orderdate, o_totalprice, sum(l_quantity)
FROM   customer c
       JOIN orders o   ON c.c_custkey = o.o_custkey
       JOIN lineitem l ON o.o_orderkey = l.l_orderkey
WHERE  o.o_orderkey IN (
       SELECT l_orderkey FROM lineitem
       GROUP  BY l_orderkey
       HAVING sum(l_quantity) > :quantity
  )
GROUP  BY c_name, c_custkey, o_orderkey, o_orderdate, o_totalprice
ORDER  BY o_totalprice DESC, o_orderdate
LIMIT 100

--+ q19
--= body
SELECT sum(l_extendedprice * (1 - l_discount)) AS revenue
FROM   lineitem l
       JOIN part p ON l.l_partkey = p.p_partkey
WHERE  (
       p.p_brand = :brand1
  AND  p.p_container IN ('SM CASE', 'SM BOX', 'SM PACK', 'SM PKG')
  AND  l.l_quantity >= :q1 AND l.l_quantity <= :q1 + 10
  AND  p.p_size BETWEEN 1 AND 5
  AND  l.l_shipmode IN ('AIR', 'AIR REG')
  AND  l.l_shipinstruct = 'DELIVER IN PERSON'
)
OR (
       p.p_brand = :brand2
  AND  p.p_container IN ('MED BAG', 'MED BOX', 'MED PKG', 'MED PACK')
  AND  l.l_quantity >= :q2 AND l.l_quantity <= :q2 + 10
  AND  p.p_size BETWEEN 1 AND 10
  AND  l.l_shipmode IN ('AIR', 'AIR REG')
  AND  l.l_shipinstruct = 'DELIVER IN PERSON'
)
OR (
       p.p_brand = :brand3
  AND  p.p_container IN ('LG CASE', 'LG BOX', 'LG PACK', 'LG PKG')
  AND  l.l_quantity >= :q3 AND l.l_quantity <= :q3 + 10
  AND  p.p_size BETWEEN 1 AND 15
  AND  l.l_shipmode IN ('AIR', 'AIR REG')
  AND  l.l_shipinstruct = 'DELIVER IN PERSON'
)

--+ q20
--= body
-- Correlated on (ps_partkey, ps_suppkey) decorrelated via JOIN-on-aggregate.
WITH lsum(l_partkey, l_suppkey, qty) AS (
    SELECT l_partkey, l_suppkey, 0.5 * sum(l_quantity)
    FROM   lineitem
    WHERE  l_shipdate >= :date
      AND  l_shipdate <  :date_1y
    GROUP  BY l_partkey, l_suppkey
)
SELECT s_name, s_address
FROM   supplier s
       JOIN nation n ON s.s_nationkey = n.n_nationkey
WHERE  s.s_suppkey IN (
       SELECT ps.ps_suppkey
       FROM   partsupp ps
              JOIN lsum ON lsum.l_partkey = ps.ps_partkey AND lsum.l_suppkey = ps.ps_suppkey
       WHERE  ps.ps_partkey IN (
              SELECT p.p_partkey FROM part p WHERE p.p_name LIKE :color || '%'
       )
         AND  ps.ps_availqty > lsum.qty
)
  AND  n.n_name = :nation
ORDER  BY s_name

--+ q21
--= body
-- Two correlated EXISTS/NOT EXISTS decorrelated: order_has_multi flags
-- orders served by ≥2 suppliers; late_per_order counts a order's distinct
-- late suppliers. "this supplier late, another supplier on the order, no
-- other late supplier" = l1 late, order in order_has_multi, and exactly one
-- late supplier on the order (which is l1). count(*) is cast: sbroad's
-- count returns unsigned and ORDER BY on an unsigned column is rejected.
WITH order_has_multi(orderkey) AS (
    SELECT l_orderkey FROM lineitem
    GROUP  BY l_orderkey
    HAVING count(DISTINCT l_suppkey) > 1
),
late_per_order(orderkey, late_suppliers) AS (
    SELECT l_orderkey, cast(count(DISTINCT l_suppkey) AS integer)
    FROM   lineitem
    WHERE  l_receiptdate > l_commitdate
    GROUP  BY l_orderkey
)
SELECT s_name, cast(count(*) AS integer) AS numwait
FROM   supplier s
       JOIN lineitem l1        ON s.s_suppkey = l1.l_suppkey
       JOIN orders o           ON o.o_orderkey = l1.l_orderkey
       JOIN nation n           ON s.s_nationkey = n.n_nationkey
       JOIN order_has_multi m  ON m.orderkey = l1.l_orderkey
       JOIN late_per_order lp  ON lp.orderkey = l1.l_orderkey
WHERE  o.o_orderstatus = 'F'
  AND  l1.l_receiptdate > l1.l_commitdate
  AND  lp.late_suppliers = 1
  AND  n.n_name = :nation
GROUP  BY s_name
ORDER  BY numwait DESC, s_name
LIMIT 100

--+ q22
--= body
-- Correlated NOT EXISTS → uncorrelated NOT IN (o_custkey is NOT NULL).
SELECT cntrycode, count(*) AS numcust, sum(c_acctbal) AS totacctbal
FROM (
  SELECT substring(c_phone FROM 1 FOR 2) AS cntrycode, c_acctbal
  FROM   customer
  WHERE  substring(c_phone FROM 1 FOR 2) IN
         (:cc1, :cc2, :cc3, :cc4, :cc5, :cc6, :cc7)
    AND  c_acctbal > (
         SELECT avg(c_acctbal)
         FROM   customer
         WHERE  c_acctbal > 0.00
           AND  substring(c_phone FROM 1 FOR 2) IN
                (:cc1, :cc2, :cc3, :cc4, :cc5, :cc6, :cc7)
    )
    AND  c_custkey NOT IN (SELECT o_custkey FROM orders)
) AS custsale
GROUP  BY cntrycode
ORDER  BY cntrycode
