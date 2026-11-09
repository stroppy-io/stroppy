-- TPC-H schema for Greenplum-family MPP engines (Greenplum, Greengage,
-- WarehousePG, Cloudberry). Same columns and types as pg.sql; the queries come from
-- pg.sql. What differs is physical design, following the pivotalguru/TPC-H
-- Greenplum kit (03_ddl/distribution.txt, functions.sh):
--   * lineitem and orders: hash on the order key, so their join stays local;
--     customer, part and supplier: hash on their primary key; partsupp: hash on
--     ps_partkey, co-located with part.
--   * lineitem, orders, partsupp, part, customer: append-optimized column-oriented,
--     zstd level 1. supplier is small and stays a heap.
--   * nation and region: DISTRIBUTED REPLICATED, a full copy on every segment.
-- No constraints and no indexes (the kit has none): MPP uniqueness checks need a
-- unique index on each segment and the planners prefer scans over index lookups.
-- `appendoptimized` and zstd work on Greenplum 6, 7 and Cloudberry 2.

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
CREATE TABLE region (
    r_regionkey  INTEGER         NOT NULL,
    r_name       CHAR(25)        NOT NULL,
    r_comment    VARCHAR(152)
)
DISTRIBUTED REPLICATED;
--= create_nation
CREATE TABLE nation (
    n_nationkey  INTEGER         NOT NULL,
    n_name       CHAR(25)        NOT NULL,
    n_regionkey  INTEGER         NOT NULL,
    n_comment    VARCHAR(152)
)
DISTRIBUTED REPLICATED;
--= create_part
CREATE TABLE part (
    p_partkey     BIGINT          NOT NULL,
    p_name        VARCHAR(55)     NOT NULL,
    p_mfgr        CHAR(25)        NOT NULL,
    p_brand       CHAR(10)        NOT NULL,
    p_type        VARCHAR(25)     NOT NULL,
    p_size        INTEGER         NOT NULL,
    p_container   CHAR(10)        NOT NULL,
    p_retailprice DECIMAL(12,2)   NOT NULL,
    p_comment     VARCHAR(23)     NOT NULL
)
WITH (appendoptimized=true, orientation=column, compresstype=zstd, compresslevel=1)
DISTRIBUTED BY (p_partkey);
--= create_supplier
CREATE TABLE supplier (
    s_suppkey    INTEGER          NOT NULL,
    s_name       CHAR(25)         NOT NULL,
    s_address    VARCHAR(40)      NOT NULL,
    s_nationkey  INTEGER          NOT NULL,
    s_phone      CHAR(15)         NOT NULL,
    s_acctbal    DECIMAL(12,2)    NOT NULL,
    s_comment    VARCHAR(101)     NOT NULL
)
DISTRIBUTED BY (s_suppkey);
--= create_partsupp
CREATE TABLE partsupp (
    ps_partkey    BIGINT          NOT NULL,
    ps_suppkey    INTEGER         NOT NULL,
    ps_availqty   INTEGER         NOT NULL,
    ps_supplycost DECIMAL(12,2)   NOT NULL,
    ps_comment    VARCHAR(199)    NOT NULL
)
WITH (appendoptimized=true, orientation=column, compresstype=zstd, compresslevel=1)
DISTRIBUTED BY (ps_partkey);
--= create_customer
CREATE TABLE customer (
    c_custkey     INTEGER         NOT NULL,
    c_name        VARCHAR(25)     NOT NULL,
    c_address     VARCHAR(40)     NOT NULL,
    c_nationkey   INTEGER         NOT NULL,
    c_phone       CHAR(15)        NOT NULL,
    c_acctbal     DECIMAL(12,2)   NOT NULL,
    c_mktsegment  CHAR(10)        NOT NULL,
    c_comment     VARCHAR(117)    NOT NULL
)
WITH (appendoptimized=true, orientation=column, compresstype=zstd, compresslevel=1)
DISTRIBUTED BY (c_custkey);
--= create_orders
CREATE TABLE orders (
    o_orderkey      BIGINT          NOT NULL,
    o_custkey       INTEGER         NOT NULL,
    o_orderstatus   CHAR(1)         NOT NULL,
    o_totalprice    DECIMAL(12,2)   NOT NULL,
    o_orderdate     DATE            NOT NULL,
    o_orderpriority CHAR(15)        NOT NULL,
    o_clerk         CHAR(15)        NOT NULL,
    o_shippriority  INTEGER         NOT NULL,
    o_comment       VARCHAR(79)     NOT NULL
)
WITH (appendoptimized=true, orientation=column, compresstype=zstd, compresslevel=1)
DISTRIBUTED BY (o_orderkey);
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
    l_returnflag    CHAR(1)         NOT NULL,
    l_linestatus    CHAR(1)         NOT NULL,
    l_shipdate      DATE            NOT NULL,
    l_commitdate    DATE            NOT NULL,
    l_receiptdate   DATE            NOT NULL,
    l_shipinstruct  CHAR(25)        NOT NULL,
    l_shipmode      CHAR(10)        NOT NULL,
    l_comment       VARCHAR(44)     NOT NULL
)
WITH (appendoptimized=true, orientation=column, compresstype=zstd, compresslevel=1)
DISTRIBUTED BY (l_orderkey);

--+ create_indexes
-- Intentionally empty: see the header.

--+ analyze
-- Segments sample their own rows; the coordinator merges the statistics.
--=
ANALYZE;
