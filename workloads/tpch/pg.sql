-- TPC-H workload for PostgreSQL. Schema follows TPC-H spec §1.4 Clause 1.4.
-- The 22 business queries live below, each as a named section with a single
-- `body` entry. Parameter placeholders use :name — values are pinned to
-- TPC-H spec §2.4.x defaults (see workloads/tpch/tx.ts for the mapping).

--+ drop_schema
--= drop_lineitem
DROP TABLE IF EXISTS lineitem;
--= drop_partsupp
DROP TABLE IF EXISTS partsupp;
--= drop_orders
DROP TABLE IF EXISTS orders;
--= drop_customer
DROP TABLE IF EXISTS customer;
--= drop_supplier
DROP TABLE IF EXISTS supplier;
--= drop_part
DROP TABLE IF EXISTS part;
--= drop_nation
DROP TABLE IF EXISTS nation;
--= drop_region
DROP TABLE IF EXISTS region;

--+ create_schema
--= create_region
CREATE UNLOGGED TABLE region (
    r_regionkey  INTEGER         NOT NULL,
    r_name       CHAR(25)        NOT NULL,
    r_comment    VARCHAR(152),
    PRIMARY KEY (r_regionkey)
);
--= create_nation
CREATE UNLOGGED TABLE nation (
    n_nationkey  INTEGER         NOT NULL,
    n_name       CHAR(25)        NOT NULL,
    n_regionkey  INTEGER         NOT NULL,
    n_comment    VARCHAR(152),
    PRIMARY KEY (n_nationkey)
);
--= create_part
CREATE UNLOGGED TABLE part (
    p_partkey     BIGINT          NOT NULL,
    p_name        VARCHAR(55)     NOT NULL,
    p_mfgr        CHAR(25)        NOT NULL,
    p_brand       CHAR(10)        NOT NULL,
    p_type        VARCHAR(25)     NOT NULL,
    p_size        INTEGER         NOT NULL,
    p_container   CHAR(10)        NOT NULL,
    p_retailprice DECIMAL(12,2)   NOT NULL,
    p_comment     VARCHAR(23)     NOT NULL,
    PRIMARY KEY (p_partkey)
);
--= create_supplier
CREATE UNLOGGED TABLE supplier (
    s_suppkey    INTEGER          NOT NULL,
    s_name       CHAR(25)         NOT NULL,
    s_address    VARCHAR(40)      NOT NULL,
    s_nationkey  INTEGER          NOT NULL,
    s_phone      CHAR(15)         NOT NULL,
    s_acctbal    DECIMAL(12,2)    NOT NULL,
    s_comment    VARCHAR(101)     NOT NULL,
    PRIMARY KEY (s_suppkey)
);
--= create_partsupp
CREATE UNLOGGED TABLE partsupp (
    ps_partkey    BIGINT          NOT NULL,
    ps_suppkey    INTEGER         NOT NULL,
    ps_availqty   INTEGER         NOT NULL,
    ps_supplycost DECIMAL(12,2)   NOT NULL,
    ps_comment    VARCHAR(199)    NOT NULL,
    PRIMARY KEY (ps_partkey, ps_suppkey)
);
--= create_customer
CREATE UNLOGGED TABLE customer (
    c_custkey     INTEGER         NOT NULL,
    c_name        VARCHAR(25)     NOT NULL,
    c_address     VARCHAR(40)     NOT NULL,
    c_nationkey   INTEGER         NOT NULL,
    c_phone       CHAR(15)        NOT NULL,
    c_acctbal     DECIMAL(12,2)   NOT NULL,
    c_mktsegment  CHAR(10)        NOT NULL,
    c_comment     VARCHAR(117)    NOT NULL,
    PRIMARY KEY (c_custkey)
);
--= create_orders
CREATE UNLOGGED TABLE orders (
    o_orderkey      BIGINT          NOT NULL,
    o_custkey       INTEGER         NOT NULL,
    o_orderstatus   CHAR(1)         NOT NULL,
    o_totalprice    DECIMAL(12,2)   NOT NULL,
    o_orderdate     DATE            NOT NULL,
    o_orderpriority CHAR(15)        NOT NULL,
    o_clerk         CHAR(15)        NOT NULL,
    o_shippriority  INTEGER         NOT NULL,
    o_comment       VARCHAR(79)     NOT NULL,
    PRIMARY KEY (o_orderkey)
);
--= create_lineitem
CREATE UNLOGGED TABLE lineitem (
    l_orderkey      BIGINT          NOT NULL,
    l_partkey       BIGINT          NOT NULL,
    l_suppkey       INTEGER         NOT NULL,
    l_linenumber    INTEGER         NOT NULL,
    l_quantity      DECIMAL(12,2)   NOT NULL,
    l_extendedprice DECIMAL(12,2)   NOT NULL,
    l_discount      DECIMAL(12,2)   NOT NULL,
    l_tax           DECIMAL(12,2)   NOT NULL,
    l_returnflag    CHAR(1)         NOT NULL,
    l_linestatus    CHAR(1)         NOT NULL,
    l_shipdate      DATE            NOT NULL,
    l_commitdate    DATE            NOT NULL,
    l_receiptdate   DATE            NOT NULL,
    l_shipinstruct  CHAR(25)        NOT NULL,
    l_shipmode      CHAR(10)        NOT NULL,
    l_comment       VARCHAR(44)     NOT NULL,
    PRIMARY KEY (l_orderkey, l_linenumber)
);

--+ set_logged
-- Flip tables from UNLOGGED (fast bulk-load on tmpfs / fsync-off pg) back to
-- LOGGED once population completes. ANALYZE afterward so the planner picks
-- sane plans for Q20/Q21 instead of nested-loop-hanging.
--= region
ALTER TABLE region   SET LOGGED;
--= nation
ALTER TABLE nation   SET LOGGED;
--= part
ALTER TABLE part     SET LOGGED;
--= supplier
ALTER TABLE supplier SET LOGGED;
--= partsupp
ALTER TABLE partsupp SET LOGGED;
--= customer
ALTER TABLE customer SET LOGGED;
--= orders
ALTER TABLE orders   SET LOGGED;
--= lineitem
ALTER TABLE lineitem SET LOGGED;
--= analyze
ANALYZE;

--+ create_indexes
--= idx_supplier_nationkey
CREATE INDEX idx_supplier_nationkey ON supplier (s_nationkey);
--= idx_partsupp_partkey
CREATE INDEX idx_partsupp_partkey    ON partsupp (ps_partkey);
--= idx_partsupp_suppkey
CREATE INDEX idx_partsupp_suppkey    ON partsupp (ps_suppkey);
--= idx_customer_nationkey
CREATE INDEX idx_customer_nationkey  ON customer (c_nationkey);
--= idx_orders_custkey
CREATE INDEX idx_orders_custkey      ON orders   (o_custkey);
--= idx_lineitem_partkey
CREATE INDEX idx_lineitem_partkey    ON lineitem (l_partkey);
--= idx_lineitem_suppkey
CREATE INDEX idx_lineitem_suppkey    ON lineitem (l_suppkey);
--= idx_lineitem_orderkey
CREATE INDEX idx_lineitem_orderkey   ON lineitem (l_orderkey);
--= idx_nation_regionkey
CREATE INDEX idx_nation_regionkey    ON nation   (n_regionkey);
--= idx_lineitem_shipdate
CREATE INDEX idx_lineitem_shipdate   ON lineitem (l_shipdate);
--= idx_orders_orderdate
CREATE INDEX idx_orders_orderdate    ON orders   (o_orderdate);

-- ==========================================================================
-- 22 TPC-H queries. Parameters follow §2.4.x defaults — see workloads/tpch/
-- tx.ts for the bound values (delta=90, region='ASIA', segment='BUILDING',
-- etc.). Each section's `body` entry holds the full SELECT text.
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
WHERE  l_shipdate <= date '1998-12-01' - (:delta * interval '1 day')
GROUP  BY l_returnflag, l_linestatus
ORDER  BY l_returnflag, l_linestatus;

--+ q2
--= body
SELECT s_acctbal, s_name, n_name, p_partkey, p_mfgr, s_address, s_phone, s_comment
FROM   part, supplier, partsupp, nation, region
WHERE  p_partkey = ps_partkey
  AND  s_suppkey = ps_suppkey
  AND  p_size   = :size
  AND  p_type LIKE '%' || :type
  AND  s_nationkey = n_nationkey
  AND  n_regionkey = r_regionkey
  AND  r_name = :region
  AND  ps_supplycost = (
       SELECT min(ps_supplycost)
       FROM   partsupp, supplier, nation, region
       WHERE  p_partkey = ps_partkey
         AND  s_suppkey = ps_suppkey
         AND  s_nationkey = n_nationkey
         AND  n_regionkey = r_regionkey
         AND  r_name = :region
  )
ORDER BY s_acctbal DESC, n_name, s_name, p_partkey
LIMIT 100;

--+ q3
--= body
SELECT l_orderkey,
       sum(l_extendedprice * (1 - l_discount)) AS revenue,
       o_orderdate,
       o_shippriority
FROM   customer, orders, lineitem
WHERE  c_mktsegment = :segment
  AND  c_custkey = o_custkey
  AND  l_orderkey = o_orderkey
  AND  o_orderdate < :date::date
  AND  l_shipdate  > :date::date
GROUP  BY l_orderkey, o_orderdate, o_shippriority
ORDER  BY revenue DESC, o_orderdate
LIMIT 10;

--+ q4
--= body
SELECT o_orderpriority, count(*) AS order_count
FROM   orders
WHERE  o_orderdate >= :date::date
  AND  o_orderdate <  :date::date + interval '3 months'
  AND  EXISTS (SELECT * FROM lineitem
               WHERE l_orderkey = o_orderkey
                 AND l_commitdate < l_receiptdate)
GROUP  BY o_orderpriority
ORDER  BY o_orderpriority;

--+ q5
--= body
SELECT n_name, sum(l_extendedprice * (1 - l_discount)) AS revenue
FROM   customer, orders, lineitem, supplier, nation, region
WHERE  c_custkey = o_custkey
  AND  l_orderkey = o_orderkey
  AND  l_suppkey = s_suppkey
  AND  c_nationkey = s_nationkey
  AND  s_nationkey = n_nationkey
  AND  n_regionkey = r_regionkey
  AND  r_name = :region
  AND  o_orderdate >= :date::date
  AND  o_orderdate <  :date::date + interval '1 year'
GROUP  BY n_name
ORDER  BY revenue DESC;

--+ q6
--= body
SELECT sum(l_extendedprice * l_discount) AS revenue
FROM   lineitem
WHERE  l_shipdate >= :date::date
  AND  l_shipdate <  :date::date + interval '1 year'
  AND  l_discount BETWEEN :discount - 0.01 AND :discount + 0.01
  AND  l_quantity < :quantity;

--+ q7
--= body
SELECT supp_nation, cust_nation, l_year, sum(volume) AS revenue
FROM (
  SELECT n1.n_name AS supp_nation,
         n2.n_name AS cust_nation,
         extract(year FROM l_shipdate) AS l_year,
         l_extendedprice * (1 - l_discount) AS volume
  FROM   supplier, lineitem, orders, customer, nation n1, nation n2
  WHERE  s_suppkey = l_suppkey
    AND  o_orderkey = l_orderkey
    AND  c_custkey = o_custkey
    AND  s_nationkey = n1.n_nationkey
    AND  c_nationkey = n2.n_nationkey
    AND  ( (n1.n_name = :nation1 AND n2.n_name = :nation2)
        OR (n1.n_name = :nation2 AND n2.n_name = :nation1))
    AND  l_shipdate BETWEEN date '1995-01-01' AND date '1996-12-31'
) AS shipping
GROUP  BY supp_nation, cust_nation, l_year
ORDER  BY supp_nation, cust_nation, l_year;

--+ q8
--= body
SELECT o_year,
       sum(CASE WHEN nation = :nation THEN volume ELSE 0 END) / sum(volume) AS mkt_share
FROM (
  SELECT extract(year FROM o_orderdate) AS o_year,
         l_extendedprice * (1 - l_discount) AS volume,
         n2.n_name AS nation
  FROM   part, supplier, lineitem, orders, customer, nation n1, nation n2, region
  WHERE  p_partkey = l_partkey
    AND  s_suppkey = l_suppkey
    AND  l_orderkey = o_orderkey
    AND  o_custkey = c_custkey
    AND  c_nationkey = n1.n_nationkey
    AND  n1.n_regionkey = r_regionkey
    AND  r_name = :region
    AND  s_nationkey = n2.n_nationkey
    AND  o_orderdate BETWEEN date '1995-01-01' AND date '1996-12-31'
    AND  p_type = :type
) AS all_nations
GROUP  BY o_year
ORDER  BY o_year;

--+ q9
--= body
SELECT nation, o_year, sum(amount) AS sum_profit
FROM (
  SELECT n_name AS nation,
         extract(year FROM o_orderdate) AS o_year,
         l_extendedprice * (1 - l_discount) - ps_supplycost * l_quantity AS amount
  FROM   part, supplier, lineitem, partsupp, orders, nation
  WHERE  s_suppkey = l_suppkey
    AND  ps_suppkey = l_suppkey
    AND  ps_partkey = l_partkey
    AND  p_partkey = l_partkey
    AND  o_orderkey = l_orderkey
    AND  s_nationkey = n_nationkey
    AND  p_name LIKE '%' || :color || '%'
) AS profit
GROUP  BY nation, o_year
ORDER  BY nation, o_year DESC;

--+ q10
--= body
SELECT c_custkey, c_name,
       sum(l_extendedprice * (1 - l_discount)) AS revenue,
       c_acctbal, n_name, c_address, c_phone, c_comment
FROM   customer, orders, lineitem, nation
WHERE  c_custkey = o_custkey
  AND  l_orderkey = o_orderkey
  AND  o_orderdate >= :date::date
  AND  o_orderdate <  :date::date + interval '3 months'
  AND  l_returnflag = 'R'
  AND  c_nationkey = n_nationkey
GROUP  BY c_custkey, c_name, c_acctbal, c_phone, n_name, c_address, c_comment
ORDER  BY revenue DESC
LIMIT 20;

--+ q11
--= body
SELECT ps_partkey, sum(ps_supplycost * ps_availqty) AS value
FROM   partsupp, supplier, nation
WHERE  ps_suppkey = s_suppkey
  AND  s_nationkey = n_nationkey
  AND  n_name = :nation
GROUP  BY ps_partkey
HAVING sum(ps_supplycost * ps_availqty) > (
       SELECT sum(ps_supplycost * ps_availqty) * :fraction
       FROM   partsupp, supplier, nation
       WHERE  ps_suppkey = s_suppkey
         AND  s_nationkey = n_nationkey
         AND  n_name = :nation
)
ORDER  BY value DESC;

--+ q12
--= body
SELECT l_shipmode,
       sum(CASE WHEN o_orderpriority = '1-URGENT'
                 OR o_orderpriority = '2-HIGH'
                THEN 1 ELSE 0 END) AS high_line_count,
       sum(CASE WHEN o_orderpriority <> '1-URGENT'
                AND o_orderpriority <> '2-HIGH'
                THEN 1 ELSE 0 END) AS low_line_count
FROM   orders, lineitem
WHERE  o_orderkey = l_orderkey
  AND  l_shipmode IN (:shipmode1, :shipmode2)
  AND  l_commitdate < l_receiptdate
  AND  l_shipdate   < l_commitdate
  AND  l_receiptdate >= :date::date
  AND  l_receiptdate <  :date::date + interval '1 year'
GROUP  BY l_shipmode
ORDER  BY l_shipmode;

--+ q13
--= body
SELECT c_count, count(*) AS custdist
FROM (
  SELECT c_custkey, count(o_orderkey) AS c_count
  FROM   customer LEFT OUTER JOIN orders
                  ON c_custkey = o_custkey
                 AND o_comment NOT LIKE '%' || :word1 || '%' || :word2 || '%'
  GROUP  BY c_custkey
) AS c_orders
GROUP  BY c_count
ORDER  BY custdist DESC, c_count DESC;

--+ q14
--= body
SELECT 100.00 * sum(CASE WHEN p_type LIKE 'PROMO%'
                         THEN l_extendedprice * (1 - l_discount)
                         ELSE 0 END)
               / sum(l_extendedprice * (1 - l_discount)) AS promo_revenue
FROM   lineitem, part
WHERE  l_partkey = p_partkey
  AND  l_shipdate >= :date::date
  AND  l_shipdate <  :date::date + interval '1 month';

--+ q15
--= body
WITH revenue(supplier_no, total_revenue) AS (
    SELECT l_suppkey, sum(l_extendedprice * (1 - l_discount))
    FROM   lineitem
    WHERE  l_shipdate >= :date::date
      AND  l_shipdate <  :date::date + interval '3 months'
    GROUP  BY l_suppkey
)
SELECT s_suppkey, s_name, s_address, s_phone, total_revenue
FROM   supplier, revenue
WHERE  s_suppkey = supplier_no
  AND  total_revenue = (SELECT max(total_revenue) FROM revenue)
ORDER  BY s_suppkey;

--+ q16
--= body
SELECT p_brand, p_type, p_size, count(DISTINCT ps_suppkey) AS supplier_cnt
FROM   partsupp, part
WHERE  p_partkey = ps_partkey
  AND  p_brand <> :brand
  AND  p_type NOT LIKE :type_prefix || '%'
  AND  p_size IN (:s1, :s2, :s3, :s4, :s5, :s6, :s7, :s8)
  AND  ps_suppkey NOT IN (
       SELECT s_suppkey FROM supplier
       WHERE  s_comment LIKE '%Customer%Complaints%'
  )
GROUP  BY p_brand, p_type, p_size
ORDER  BY supplier_cnt DESC, p_brand, p_type, p_size;

--+ q17
--= body
SELECT sum(l_extendedprice) / 7.0 AS avg_yearly
FROM   lineitem, part
WHERE  p_partkey = l_partkey
  AND  p_brand = :brand
  AND  p_container = :container
  AND  l_quantity < (
       SELECT 0.2 * avg(l_quantity)
       FROM   lineitem
       WHERE  l_partkey = p_partkey
  );

--+ q18
--= body
SELECT c_name, c_custkey, o_orderkey, o_orderdate, o_totalprice, sum(l_quantity)
FROM   customer, orders, lineitem
WHERE  o_orderkey IN (
       SELECT l_orderkey FROM lineitem
       GROUP  BY l_orderkey
       HAVING sum(l_quantity) > :quantity
  )
  AND  c_custkey = o_custkey
  AND  o_orderkey = l_orderkey
GROUP  BY c_name, c_custkey, o_orderkey, o_orderdate, o_totalprice
ORDER  BY o_totalprice DESC, o_orderdate
LIMIT 100;

--+ q19
--= body
SELECT sum(l_extendedprice * (1 - l_discount)) AS revenue
FROM   lineitem, part
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
);

--+ q20
--= body
SELECT s_name, s_address
FROM   supplier, nation
WHERE  s_suppkey IN (
       SELECT ps_suppkey
       FROM   partsupp
       WHERE  ps_partkey IN (
              SELECT p_partkey
              FROM   part
              WHERE  p_name LIKE :color || '%'
       )
         AND  ps_availqty > (
              SELECT 0.5 * sum(l_quantity)
              FROM   lineitem
              WHERE  l_partkey = ps_partkey
                AND  l_suppkey = ps_suppkey
                AND  l_shipdate >= :date::date
                AND  l_shipdate <  :date::date + interval '1 year'
       )
)
  AND  s_nationkey = n_nationkey
  AND  n_name = :nation
ORDER  BY s_name;

--+ q21
--= body
SELECT s_name, count(*) AS numwait
FROM   supplier, lineitem l1, orders, nation
WHERE  s_suppkey = l1.l_suppkey
  AND  o_orderkey = l1.l_orderkey
  AND  o_orderstatus = 'F'
  AND  l1.l_receiptdate > l1.l_commitdate
  AND  EXISTS (SELECT * FROM lineitem l2
               WHERE l2.l_orderkey = l1.l_orderkey
                 AND l2.l_suppkey <> l1.l_suppkey)
  AND  NOT EXISTS (SELECT * FROM lineitem l3
                   WHERE l3.l_orderkey = l1.l_orderkey
                     AND l3.l_suppkey <> l1.l_suppkey
                     AND l3.l_receiptdate > l3.l_commitdate)
  AND  s_nationkey = n_nationkey
  AND  n_name = :nation
GROUP  BY s_name
ORDER  BY numwait DESC, s_name
LIMIT 100;

--+ q22
--= body
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
    AND  NOT EXISTS (SELECT * FROM orders WHERE o_custkey = c_custkey)
) AS custsale
GROUP  BY cntrycode
ORDER  BY cntrycode;
