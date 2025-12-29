-- section create_schema

-- query
CREATE TABLE pgbench_branches (
    bid INTEGER NOT NULL PRIMARY KEY,
    bbalance INTEGER,
    filler CHAR(88)
);
-- query end

-- query
CREATE TABLE pgbench_tellers (
    tid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    tbalance INTEGER,
    filler CHAR(84)
);
-- query end

-- query
CREATE TABLE pgbench_accounts (
    aid INTEGER NOT NULL PRIMARY KEY,
    bid INTEGER,
    abalance INTEGER,
    filler CHAR(84)
);
-- query end

-- query
CREATE TABLE pgbench_history (
    tid INTEGER,
    bid INTEGER,
    aid INTEGER,
    delta INTEGER,
    mtime TIMESTAMP,
    filler CHAR(22)
);
-- query end
-- section end

-- section insert

-- query
INSERT INTO pgbench_branches (bid, bbalance, filler)
SELECT
    generate_series(1, 1), -- scale factor
    0,
    REPEAT('x', 88);
-- query end

-- query
INSERT INTO pgbench_tellers (tid, bid, tbalance, filler)
SELECT
    generate_series(1, 10), -- 10 * scale factor
    ((generate_series(1, 10) - 1) / 10) + 1,
    0,
    REPEAT('x', 84);
-- query end

-- query
INSERT INTO pgbench_accounts (aid, bid, abalance, filler)
SELECT
    generate_series(1, 100000), -- 100000 * scale factor
    ((generate_series(1, 100000) - 1) / 100000) + 1,
    0,
    REPEAT('x', 84);
-- query end
-- section end

-- section workload
-- transaction
-- query end

-- query
UPDATE pgbench_accounts
SET abalance = abalance + 
WHERE aid = ${aid};
-- query end

-- query
SELECT abalance
FROM pgbench_accounts
WHERE aid = ${aid};
-- query end

-- query
UPDATE pgbench_tellers
SET tbalance = tbalance + ${delta}
WHERE tid = ${tid};
-- query end

-- query
UPDATE pgbench_branches
SET bbalance = bbalance + ${delta}
WHERE bid = ${bid};
-- query end

-- query
INSERT INTO pgbench_history (tid, bid, aid, delta, mtime)
VALUES (${tid}, ${bid}, ${aid}, ${delta}, CURRENT_TIMESTAMP);
-- query end
-- transaction end
-- section end

-- section cleanup
-- query
DROP TABLE IF EXISTS pgbench_history CASCADE;
-- query end

-- query
DROP TABLE IF EXISTS pgbench_accounts CASCADE;
-- query end

-- query
DROP TABLE IF EXISTS pgbench_tellers CASCADE;
-- query end

-- query
DROP TABLE IF EXISTS pgbench_branches CASCADE;
-- query end
-- section end
