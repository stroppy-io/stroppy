--+ schema
--= drop
DROP TABLE simple_kv

--= create
CREATE TABLE simple_kv (
    id  bigint PRIMARY KEY,
    val text,
    num float8
)

--+ workload
--= point_select
SELECT val, num FROM simple_kv WHERE id = :id

--+ check
--= count
SELECT count(*) FROM simple_kv
