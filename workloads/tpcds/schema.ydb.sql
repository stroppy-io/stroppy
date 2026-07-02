-- TPC-DS schema for YDB (YQL). Types map the canonical TPC-DS columns to YQL:
--   integer -> Int64, char(N)/varchar(N) -> Utf8, decimal(p,s) -> Double,
--   date -> Utf8 (ISO 'YYYY-MM-DD' strings; TPC-DS date_dim spans 1900-2100,
--   outside YDB's unsigned Timestamp/Date epoch >= 1970 — strings compare
--   lexicographically so BETWEEN/= filters still hold). The ported dsdgen
--   generator emits every cell as text (pkg/datagen/tpcdsgen.normalize); the
--   ydb driver's BulkUpsert path parses those strings into the declared numeric
--   type and passes text/date strings through (pkg/driver/ydb/insert_spec.go).
--
-- YDB has no FOREIGN KEY and requires a PRIMARY KEY per table, so each table
-- uses its canonical TPC-DS key (fact tables the (item, ticket/order) pair).
-- Only PK columns are NOT NULL: TPC-DS fact foreign-key columns are genuinely
-- nullable, and BulkUpsert must accept the generated nulls.
--
-- Two storage sections, chosen by YDB_STORE_MODE in tpcds.ts:
--   create_schema        (row store, default)  keeps window functions and the
--                        full analytic query surface that YDB column store
--                        still restricts.
--   create_schema_column (column store)        OLAP layout for scan-heavy runs;
--                        some window/grouping queries may not plan.
-- Fact and large dimension tables start at 64 partitions, small dims at 1.
-- Secondary indexes are omitted (full-scan analytics; the spec lists them as
-- auxiliary) — create_indexes is a noop so the tpcds.ts pipeline step is a
-- no-op for ydb.

--+ drop_schema
--= drop_web_returns
DROP TABLE IF EXISTS web_returns
--= drop_web_sales
DROP TABLE IF EXISTS web_sales
--= drop_catalog_returns
DROP TABLE IF EXISTS catalog_returns
--= drop_catalog_sales
DROP TABLE IF EXISTS catalog_sales
--= drop_store_returns
DROP TABLE IF EXISTS store_returns
--= drop_store_sales
DROP TABLE IF EXISTS store_sales
--= drop_inventory
DROP TABLE IF EXISTS inventory
--= drop_item
DROP TABLE IF EXISTS item
--= drop_promotion
DROP TABLE IF EXISTS promotion
--= drop_store
DROP TABLE IF EXISTS store
--= drop_call_center
DROP TABLE IF EXISTS call_center
--= drop_customer
DROP TABLE IF EXISTS customer
--= drop_customer_address
DROP TABLE IF EXISTS customer_address
--= drop_catalog_page
DROP TABLE IF EXISTS catalog_page
--= drop_web_site
DROP TABLE IF EXISTS web_site
--= drop_web_page
DROP TABLE IF EXISTS web_page
--= drop_warehouse
DROP TABLE IF EXISTS warehouse
--= drop_time_dim
DROP TABLE IF EXISTS time_dim
--= drop_date_dim
DROP TABLE IF EXISTS date_dim
--= drop_customer_demographics
DROP TABLE IF EXISTS customer_demographics
--= drop_household_demographics
DROP TABLE IF EXISTS household_demographics
--= drop_reason
DROP TABLE IF EXISTS reason
--= drop_ship_mode
DROP TABLE IF EXISTS ship_mode
--= drop_income_band
DROP TABLE IF EXISTS income_band

--+ create_schema
--= income_band
CREATE TABLE income_band (
    ib_income_band_sk   Int64 NOT NULL,
    ib_lower_bound      Int64,
    ib_upper_bound      Int64,
    PRIMARY KEY (ib_income_band_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= ship_mode
CREATE TABLE ship_mode (
    sm_ship_mode_sk   Int64 NOT NULL,
    sm_ship_mode_id   Utf8,
    sm_type           Utf8,
    sm_code           Utf8,
    sm_carrier        Utf8,
    sm_contract       Utf8,
    PRIMARY KEY (sm_ship_mode_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= reason
CREATE TABLE reason (
    r_reason_sk     Int64 NOT NULL,
    r_reason_id     Utf8,
    r_reason_desc   Utf8,
    PRIMARY KEY (r_reason_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= household_demographics
CREATE TABLE household_demographics (
    hd_demo_sk          Int64 NOT NULL,
    hd_income_band_sk   Int64,
    hd_buy_potential    Utf8,
    hd_dep_count        Int64,
    hd_vehicle_count    Int64,
    PRIMARY KEY (hd_demo_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= customer_demographics
CREATE TABLE customer_demographics (
    cd_demo_sk              Int64 NOT NULL,
    cd_gender               Utf8,
    cd_marital_status       Utf8,
    cd_education_status     Utf8,
    cd_purchase_estimate    Int64,
    cd_credit_rating        Utf8,
    cd_dep_count            Int64,
    cd_dep_employed_count   Int64,
    cd_dep_college_count    Int64,
    PRIMARY KEY (cd_demo_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= date_dim
CREATE TABLE date_dim (
    d_date_sk             Int64 NOT NULL,
    d_date_id             Utf8,
    d_date                Utf8,
    d_month_seq           Int64,
    d_week_seq            Int64,
    d_quarter_seq         Int64,
    d_year                Int64,
    d_dow                 Int64,
    d_moy                 Int64,
    d_dom                 Int64,
    d_qoy                 Int64,
    d_fy_year             Int64,
    d_fy_quarter_seq      Int64,
    d_fy_week_seq         Int64,
    d_day_name            Utf8,
    d_quarter_name        Utf8,
    d_holiday             Utf8,
    d_weekend             Utf8,
    d_following_holiday   Utf8,
    d_first_dom           Int64,
    d_last_dom            Int64,
    d_same_day_ly         Int64,
    d_same_day_lq         Int64,
    d_current_day         Utf8,
    d_current_week        Utf8,
    d_current_month       Utf8,
    d_current_quarter     Utf8,
    d_current_year        Utf8,
    PRIMARY KEY (d_date_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= time_dim
CREATE TABLE time_dim (
    t_time_sk     Int64 NOT NULL,
    t_time_id     Utf8,
    t_time        Int64,
    t_hour        Int64,
    t_minute      Int64,
    t_second      Int64,
    t_am_pm       Utf8,
    t_shift       Utf8,
    t_sub_shift   Utf8,
    t_meal_time   Utf8,
    PRIMARY KEY (t_time_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= warehouse
CREATE TABLE warehouse (
    w_warehouse_sk      Int64 NOT NULL,
    w_warehouse_id      Utf8,
    w_warehouse_name    Utf8,
    w_warehouse_sq_ft   Int64,
    w_street_number     Utf8,
    w_street_name       Utf8,
    w_street_type       Utf8,
    w_suite_number      Utf8,
    w_city              Utf8,
    w_county            Utf8,
    w_state             Utf8,
    w_zip               Utf8,
    w_country           Utf8,
    w_gmt_offset        Double,
    PRIMARY KEY (w_warehouse_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= web_page
CREATE TABLE web_page (
    wp_web_page_sk        Int64 NOT NULL,
    wp_web_page_id        Utf8,
    wp_rec_start_date     Utf8,
    wp_rec_end_date       Utf8,
    wp_creation_date_sk   Int64,
    wp_access_date_sk     Int64,
    wp_autogen_flag       Utf8,
    wp_customer_sk        Int64,
    wp_url                Utf8,
    wp_type               Utf8,
    wp_char_count         Int64,
    wp_link_count         Int64,
    wp_image_count        Int64,
    wp_max_ad_count       Int64,
    PRIMARY KEY (wp_web_page_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= web_site
CREATE TABLE web_site (
    web_site_sk          Int64 NOT NULL,
    web_site_id          Utf8,
    web_rec_start_date   Utf8,
    web_rec_end_date     Utf8,
    web_name             Utf8,
    web_open_date_sk     Int64,
    web_close_date_sk    Int64,
    web_class            Utf8,
    web_manager          Utf8,
    web_mkt_id           Int64,
    web_mkt_class        Utf8,
    web_mkt_desc         Utf8,
    web_market_manager   Utf8,
    web_company_id       Int64,
    web_company_name     Utf8,
    web_street_number    Utf8,
    web_street_name      Utf8,
    web_street_type      Utf8,
    web_suite_number     Utf8,
    web_city             Utf8,
    web_county           Utf8,
    web_state            Utf8,
    web_zip              Utf8,
    web_country          Utf8,
    web_gmt_offset       Double,
    web_tax_percentage   Double,
    PRIMARY KEY (web_site_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= catalog_page
CREATE TABLE catalog_page (
    cp_catalog_page_sk       Int64 NOT NULL,
    cp_catalog_page_id       Utf8,
    cp_start_date_sk         Int64,
    cp_end_date_sk           Int64,
    cp_department            Utf8,
    cp_catalog_number        Int64,
    cp_catalog_page_number   Int64,
    cp_description           Utf8,
    cp_type                  Utf8,
    PRIMARY KEY (cp_catalog_page_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= customer_address
CREATE TABLE customer_address (
    ca_address_sk      Int64 NOT NULL,
    ca_address_id      Utf8,
    ca_street_number   Utf8,
    ca_street_name     Utf8,
    ca_street_type     Utf8,
    ca_suite_number    Utf8,
    ca_city            Utf8,
    ca_county          Utf8,
    ca_state           Utf8,
    ca_zip             Utf8,
    ca_country         Utf8,
    ca_gmt_offset      Double,
    ca_location_type   Utf8,
    PRIMARY KEY (ca_address_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= customer
CREATE TABLE customer (
    c_customer_sk            Int64 NOT NULL,
    c_customer_id            Utf8,
    c_current_cdemo_sk       Int64,
    c_current_hdemo_sk       Int64,
    c_current_addr_sk        Int64,
    c_first_shipto_date_sk   Int64,
    c_first_sales_date_sk    Int64,
    c_salutation             Utf8,
    c_first_name             Utf8,
    c_last_name              Utf8,
    c_preferred_cust_flag    Utf8,
    c_birth_day              Int64,
    c_birth_month            Int64,
    c_birth_year             Int64,
    c_birth_country          Utf8,
    c_login                  Utf8,
    c_email_address          Utf8,
    c_last_review_date       Utf8,
    PRIMARY KEY (c_customer_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= call_center
CREATE TABLE call_center (
    cc_call_center_sk   Int64 NOT NULL,
    cc_call_center_id   Utf8,
    cc_rec_start_date   Utf8,
    cc_rec_end_date     Utf8,
    cc_closed_date_sk   Int64,
    cc_open_date_sk     Int64,
    cc_name             Utf8,
    cc_class            Utf8,
    cc_employees        Int64,
    cc_sq_ft            Int64,
    cc_hours            Utf8,
    cc_manager          Utf8,
    cc_mkt_id           Int64,
    cc_mkt_class        Utf8,
    cc_mkt_desc         Utf8,
    cc_market_manager   Utf8,
    cc_division         Int64,
    cc_division_name    Utf8,
    cc_company          Int64,
    cc_company_name     Utf8,
    cc_street_number    Utf8,
    cc_street_name      Utf8,
    cc_street_type      Utf8,
    cc_suite_number     Utf8,
    cc_city             Utf8,
    cc_county           Utf8,
    cc_state            Utf8,
    cc_zip              Utf8,
    cc_country          Utf8,
    cc_gmt_offset       Double,
    cc_tax_percentage   Double,
    PRIMARY KEY (cc_call_center_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= store
CREATE TABLE store (
    s_store_sk           Int64 NOT NULL,
    s_store_id           Utf8,
    s_rec_start_date     Utf8,
    s_rec_end_date       Utf8,
    s_closed_date_sk     Int64,
    s_store_name         Utf8,
    s_number_employees   Int64,
    s_floor_space        Int64,
    s_hours              Utf8,
    s_manager            Utf8,
    s_market_id          Int64,
    s_geography_class    Utf8,
    s_market_desc        Utf8,
    s_market_manager     Utf8,
    s_division_id        Int64,
    s_division_name      Utf8,
    s_company_id         Int64,
    s_company_name       Utf8,
    s_street_number      Utf8,
    s_street_name        Utf8,
    s_street_type        Utf8,
    s_suite_number       Utf8,
    s_city               Utf8,
    s_county             Utf8,
    s_state              Utf8,
    s_zip                Utf8,
    s_country            Utf8,
    s_gmt_offset         Double,
    s_tax_precentage     Double,
    PRIMARY KEY (s_store_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= promotion
CREATE TABLE promotion (
    p_promo_sk          Int64 NOT NULL,
    p_promo_id          Utf8,
    p_start_date_sk     Int64,
    p_end_date_sk       Int64,
    p_item_sk           Int64,
    p_cost              Double,
    p_response_target   Int64,
    p_promo_name        Utf8,
    p_channel_dmail     Utf8,
    p_channel_email     Utf8,
    p_channel_catalog   Utf8,
    p_channel_tv        Utf8,
    p_channel_radio     Utf8,
    p_channel_press     Utf8,
    p_channel_event     Utf8,
    p_channel_demo      Utf8,
    p_channel_details   Utf8,
    p_purpose           Utf8,
    p_discount_active   Utf8,
    PRIMARY KEY (p_promo_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= item
CREATE TABLE item (
    i_item_sk          Int64 NOT NULL,
    i_item_id          Utf8,
    i_rec_start_date   Utf8,
    i_rec_end_date     Utf8,
    i_item_desc        Utf8,
    i_current_price    Double,
    i_wholesale_cost   Double,
    i_brand_id         Int64,
    i_brand            Utf8,
    i_class_id         Int64,
    i_class            Utf8,
    i_category_id      Int64,
    i_category         Utf8,
    i_manufact_id      Int64,
    i_manufact         Utf8,
    i_size             Utf8,
    i_formulation      Utf8,
    i_color            Utf8,
    i_units            Utf8,
    i_container        Utf8,
    i_manager_id       Int64,
    i_product_name     Utf8,
    PRIMARY KEY (i_item_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= inventory
CREATE TABLE inventory (
    inv_date_sk            Int64 NOT NULL,
    inv_item_sk            Int64 NOT NULL,
    inv_warehouse_sk       Int64 NOT NULL,
    inv_quantity_on_hand   Int64,
    PRIMARY KEY (inv_date_sk, inv_item_sk, inv_warehouse_sk)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= store_sales
CREATE TABLE store_sales (
    ss_sold_date_sk         Int64,
    ss_sold_time_sk         Int64,
    ss_item_sk              Int64 NOT NULL,
    ss_customer_sk          Int64,
    ss_cdemo_sk             Int64,
    ss_hdemo_sk             Int64,
    ss_addr_sk              Int64,
    ss_store_sk             Int64,
    ss_promo_sk             Int64,
    ss_ticket_number        Int64 NOT NULL,
    ss_quantity             Int64,
    ss_wholesale_cost       Double,
    ss_list_price           Double,
    ss_sales_price          Double,
    ss_ext_discount_amt     Double,
    ss_ext_sales_price      Double,
    ss_ext_wholesale_cost   Double,
    ss_ext_list_price       Double,
    ss_ext_tax              Double,
    ss_coupon_amt           Double,
    ss_net_paid             Double,
    ss_net_paid_inc_tax     Double,
    ss_net_profit           Double,
    PRIMARY KEY (ss_item_sk, ss_ticket_number)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= store_returns
CREATE TABLE store_returns (
    sr_returned_date_sk     Int64,
    sr_return_time_sk       Int64,
    sr_item_sk              Int64 NOT NULL,
    sr_customer_sk          Int64,
    sr_cdemo_sk             Int64,
    sr_hdemo_sk             Int64,
    sr_addr_sk              Int64,
    sr_store_sk             Int64,
    sr_reason_sk            Int64,
    sr_ticket_number        Int64 NOT NULL,
    sr_return_quantity      Int64,
    sr_return_amt           Double,
    sr_return_tax           Double,
    sr_return_amt_inc_tax   Double,
    sr_fee                  Double,
    sr_return_ship_cost     Double,
    sr_refunded_cash        Double,
    sr_reversed_charge      Double,
    sr_store_credit         Double,
    sr_net_loss             Double,
    PRIMARY KEY (sr_item_sk, sr_ticket_number)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= catalog_sales
CREATE TABLE catalog_sales (
    cs_sold_date_sk            Int64,
    cs_sold_time_sk            Int64,
    cs_ship_date_sk            Int64,
    cs_bill_customer_sk        Int64,
    cs_bill_cdemo_sk           Int64,
    cs_bill_hdemo_sk           Int64,
    cs_bill_addr_sk            Int64,
    cs_ship_customer_sk        Int64,
    cs_ship_cdemo_sk           Int64,
    cs_ship_hdemo_sk           Int64,
    cs_ship_addr_sk            Int64,
    cs_call_center_sk          Int64,
    cs_catalog_page_sk         Int64,
    cs_ship_mode_sk            Int64,
    cs_warehouse_sk            Int64,
    cs_item_sk                 Int64 NOT NULL,
    cs_promo_sk                Int64,
    cs_order_number            Int64 NOT NULL,
    cs_quantity                Int64,
    cs_wholesale_cost          Double,
    cs_list_price              Double,
    cs_sales_price             Double,
    cs_ext_discount_amt        Double,
    cs_ext_sales_price         Double,
    cs_ext_wholesale_cost      Double,
    cs_ext_list_price          Double,
    cs_ext_tax                 Double,
    cs_coupon_amt              Double,
    cs_ext_ship_cost           Double,
    cs_net_paid                Double,
    cs_net_paid_inc_tax        Double,
    cs_net_paid_inc_ship       Double,
    cs_net_paid_inc_ship_tax   Double,
    cs_net_profit              Double,
    PRIMARY KEY (cs_item_sk, cs_order_number)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= catalog_returns
CREATE TABLE catalog_returns (
    cr_returned_date_sk        Int64,
    cr_returned_time_sk        Int64,
    cr_item_sk                 Int64 NOT NULL,
    cr_refunded_customer_sk    Int64,
    cr_refunded_cdemo_sk       Int64,
    cr_refunded_hdemo_sk       Int64,
    cr_refunded_addr_sk        Int64,
    cr_returning_customer_sk   Int64,
    cr_returning_cdemo_sk      Int64,
    cr_returning_hdemo_sk      Int64,
    cr_returning_addr_sk       Int64,
    cr_call_center_sk          Int64,
    cr_catalog_page_sk         Int64,
    cr_ship_mode_sk            Int64,
    cr_warehouse_sk            Int64,
    cr_reason_sk               Int64,
    cr_order_number            Int64 NOT NULL,
    cr_return_quantity         Int64,
    cr_return_amount           Double,
    cr_return_tax              Double,
    cr_return_amt_inc_tax      Double,
    cr_fee                     Double,
    cr_return_ship_cost        Double,
    cr_refunded_cash           Double,
    cr_reversed_charge         Double,
    cr_store_credit            Double,
    cr_net_loss                Double,
    PRIMARY KEY (cr_item_sk, cr_order_number)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= web_sales
CREATE TABLE web_sales (
    ws_sold_date_sk            Int64,
    ws_sold_time_sk            Int64,
    ws_ship_date_sk            Int64,
    ws_item_sk                 Int64 NOT NULL,
    ws_bill_customer_sk        Int64,
    ws_bill_cdemo_sk           Int64,
    ws_bill_hdemo_sk           Int64,
    ws_bill_addr_sk            Int64,
    ws_ship_customer_sk        Int64,
    ws_ship_cdemo_sk           Int64,
    ws_ship_hdemo_sk           Int64,
    ws_ship_addr_sk            Int64,
    ws_web_page_sk             Int64,
    ws_web_site_sk             Int64,
    ws_ship_mode_sk            Int64,
    ws_warehouse_sk            Int64,
    ws_promo_sk                Int64,
    ws_order_number            Int64 NOT NULL,
    ws_quantity                Int64,
    ws_wholesale_cost          Double,
    ws_list_price              Double,
    ws_sales_price             Double,
    ws_ext_discount_amt        Double,
    ws_ext_sales_price         Double,
    ws_ext_wholesale_cost      Double,
    ws_ext_list_price          Double,
    ws_ext_tax                 Double,
    ws_coupon_amt              Double,
    ws_ext_ship_cost           Double,
    ws_net_paid                Double,
    ws_net_paid_inc_tax        Double,
    ws_net_paid_inc_ship       Double,
    ws_net_paid_inc_ship_tax   Double,
    ws_net_profit              Double,
    PRIMARY KEY (ws_item_sk, ws_order_number)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= web_returns
CREATE TABLE web_returns (
    wr_returned_date_sk        Int64,
    wr_returned_time_sk        Int64,
    wr_item_sk                 Int64 NOT NULL,
    wr_refunded_customer_sk    Int64,
    wr_refunded_cdemo_sk       Int64,
    wr_refunded_hdemo_sk       Int64,
    wr_refunded_addr_sk        Int64,
    wr_returning_customer_sk   Int64,
    wr_returning_cdemo_sk      Int64,
    wr_returning_hdemo_sk      Int64,
    wr_returning_addr_sk       Int64,
    wr_web_page_sk             Int64,
    wr_reason_sk               Int64,
    wr_order_number            Int64 NOT NULL,
    wr_return_quantity         Int64,
    wr_return_amt              Double,
    wr_return_tax              Double,
    wr_return_amt_inc_tax      Double,
    wr_fee                     Double,
    wr_return_ship_cost        Double,
    wr_refunded_cash           Double,
    wr_reversed_charge         Double,
    wr_account_credit          Double,
    wr_net_loss                Double,
    PRIMARY KEY (wr_item_sk, wr_order_number)
)
WITH (
    STORE = ROW,
    AUTO_PARTITIONING_PARTITION_SIZE_MB = 2000,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)

--+ create_schema_column
--= income_band
CREATE TABLE income_band (
    ib_income_band_sk   Int64 NOT NULL,
    ib_lower_bound      Int64,
    ib_upper_bound      Int64,
    PRIMARY KEY (ib_income_band_sk)
)
PARTITION BY HASH (ib_income_band_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= ship_mode
CREATE TABLE ship_mode (
    sm_ship_mode_sk   Int64 NOT NULL,
    sm_ship_mode_id   Utf8,
    sm_type           Utf8,
    sm_code           Utf8,
    sm_carrier        Utf8,
    sm_contract       Utf8,
    PRIMARY KEY (sm_ship_mode_sk)
)
PARTITION BY HASH (sm_ship_mode_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= reason
CREATE TABLE reason (
    r_reason_sk     Int64 NOT NULL,
    r_reason_id     Utf8,
    r_reason_desc   Utf8,
    PRIMARY KEY (r_reason_sk)
)
PARTITION BY HASH (r_reason_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= household_demographics
CREATE TABLE household_demographics (
    hd_demo_sk          Int64 NOT NULL,
    hd_income_band_sk   Int64,
    hd_buy_potential    Utf8,
    hd_dep_count        Int64,
    hd_vehicle_count    Int64,
    PRIMARY KEY (hd_demo_sk)
)
PARTITION BY HASH (hd_demo_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= customer_demographics
CREATE TABLE customer_demographics (
    cd_demo_sk              Int64 NOT NULL,
    cd_gender               Utf8,
    cd_marital_status       Utf8,
    cd_education_status     Utf8,
    cd_purchase_estimate    Int64,
    cd_credit_rating        Utf8,
    cd_dep_count            Int64,
    cd_dep_employed_count   Int64,
    cd_dep_college_count    Int64,
    PRIMARY KEY (cd_demo_sk)
)
PARTITION BY HASH (cd_demo_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= date_dim
CREATE TABLE date_dim (
    d_date_sk             Int64 NOT NULL,
    d_date_id             Utf8,
    d_date                Utf8,
    d_month_seq           Int64,
    d_week_seq            Int64,
    d_quarter_seq         Int64,
    d_year                Int64,
    d_dow                 Int64,
    d_moy                 Int64,
    d_dom                 Int64,
    d_qoy                 Int64,
    d_fy_year             Int64,
    d_fy_quarter_seq      Int64,
    d_fy_week_seq         Int64,
    d_day_name            Utf8,
    d_quarter_name        Utf8,
    d_holiday             Utf8,
    d_weekend             Utf8,
    d_following_holiday   Utf8,
    d_first_dom           Int64,
    d_last_dom            Int64,
    d_same_day_ly         Int64,
    d_same_day_lq         Int64,
    d_current_day         Utf8,
    d_current_week        Utf8,
    d_current_month       Utf8,
    d_current_quarter     Utf8,
    d_current_year        Utf8,
    PRIMARY KEY (d_date_sk)
)
PARTITION BY HASH (d_date_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= time_dim
CREATE TABLE time_dim (
    t_time_sk     Int64 NOT NULL,
    t_time_id     Utf8,
    t_time        Int64,
    t_hour        Int64,
    t_minute      Int64,
    t_second      Int64,
    t_am_pm       Utf8,
    t_shift       Utf8,
    t_sub_shift   Utf8,
    t_meal_time   Utf8,
    PRIMARY KEY (t_time_sk)
)
PARTITION BY HASH (t_time_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= warehouse
CREATE TABLE warehouse (
    w_warehouse_sk      Int64 NOT NULL,
    w_warehouse_id      Utf8,
    w_warehouse_name    Utf8,
    w_warehouse_sq_ft   Int64,
    w_street_number     Utf8,
    w_street_name       Utf8,
    w_street_type       Utf8,
    w_suite_number      Utf8,
    w_city              Utf8,
    w_county            Utf8,
    w_state             Utf8,
    w_zip               Utf8,
    w_country           Utf8,
    w_gmt_offset        Double,
    PRIMARY KEY (w_warehouse_sk)
)
PARTITION BY HASH (w_warehouse_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= web_page
CREATE TABLE web_page (
    wp_web_page_sk        Int64 NOT NULL,
    wp_web_page_id        Utf8,
    wp_rec_start_date     Utf8,
    wp_rec_end_date       Utf8,
    wp_creation_date_sk   Int64,
    wp_access_date_sk     Int64,
    wp_autogen_flag       Utf8,
    wp_customer_sk        Int64,
    wp_url                Utf8,
    wp_type               Utf8,
    wp_char_count         Int64,
    wp_link_count         Int64,
    wp_image_count        Int64,
    wp_max_ad_count       Int64,
    PRIMARY KEY (wp_web_page_sk)
)
PARTITION BY HASH (wp_web_page_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= web_site
CREATE TABLE web_site (
    web_site_sk          Int64 NOT NULL,
    web_site_id          Utf8,
    web_rec_start_date   Utf8,
    web_rec_end_date     Utf8,
    web_name             Utf8,
    web_open_date_sk     Int64,
    web_close_date_sk    Int64,
    web_class            Utf8,
    web_manager          Utf8,
    web_mkt_id           Int64,
    web_mkt_class        Utf8,
    web_mkt_desc         Utf8,
    web_market_manager   Utf8,
    web_company_id       Int64,
    web_company_name     Utf8,
    web_street_number    Utf8,
    web_street_name      Utf8,
    web_street_type      Utf8,
    web_suite_number     Utf8,
    web_city             Utf8,
    web_county           Utf8,
    web_state            Utf8,
    web_zip              Utf8,
    web_country          Utf8,
    web_gmt_offset       Double,
    web_tax_percentage   Double,
    PRIMARY KEY (web_site_sk)
)
PARTITION BY HASH (web_site_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= catalog_page
CREATE TABLE catalog_page (
    cp_catalog_page_sk       Int64 NOT NULL,
    cp_catalog_page_id       Utf8,
    cp_start_date_sk         Int64,
    cp_end_date_sk           Int64,
    cp_department            Utf8,
    cp_catalog_number        Int64,
    cp_catalog_page_number   Int64,
    cp_description           Utf8,
    cp_type                  Utf8,
    PRIMARY KEY (cp_catalog_page_sk)
)
PARTITION BY HASH (cp_catalog_page_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= customer_address
CREATE TABLE customer_address (
    ca_address_sk      Int64 NOT NULL,
    ca_address_id      Utf8,
    ca_street_number   Utf8,
    ca_street_name     Utf8,
    ca_street_type     Utf8,
    ca_suite_number    Utf8,
    ca_city            Utf8,
    ca_county          Utf8,
    ca_state           Utf8,
    ca_zip             Utf8,
    ca_country         Utf8,
    ca_gmt_offset      Double,
    ca_location_type   Utf8,
    PRIMARY KEY (ca_address_sk)
)
PARTITION BY HASH (ca_address_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= customer
CREATE TABLE customer (
    c_customer_sk            Int64 NOT NULL,
    c_customer_id            Utf8,
    c_current_cdemo_sk       Int64,
    c_current_hdemo_sk       Int64,
    c_current_addr_sk        Int64,
    c_first_shipto_date_sk   Int64,
    c_first_sales_date_sk    Int64,
    c_salutation             Utf8,
    c_first_name             Utf8,
    c_last_name              Utf8,
    c_preferred_cust_flag    Utf8,
    c_birth_day              Int64,
    c_birth_month            Int64,
    c_birth_year             Int64,
    c_birth_country          Utf8,
    c_login                  Utf8,
    c_email_address          Utf8,
    c_last_review_date       Utf8,
    PRIMARY KEY (c_customer_sk)
)
PARTITION BY HASH (c_customer_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= call_center
CREATE TABLE call_center (
    cc_call_center_sk   Int64 NOT NULL,
    cc_call_center_id   Utf8,
    cc_rec_start_date   Utf8,
    cc_rec_end_date     Utf8,
    cc_closed_date_sk   Int64,
    cc_open_date_sk     Int64,
    cc_name             Utf8,
    cc_class            Utf8,
    cc_employees        Int64,
    cc_sq_ft            Int64,
    cc_hours            Utf8,
    cc_manager          Utf8,
    cc_mkt_id           Int64,
    cc_mkt_class        Utf8,
    cc_mkt_desc         Utf8,
    cc_market_manager   Utf8,
    cc_division         Int64,
    cc_division_name    Utf8,
    cc_company          Int64,
    cc_company_name     Utf8,
    cc_street_number    Utf8,
    cc_street_name      Utf8,
    cc_street_type      Utf8,
    cc_suite_number     Utf8,
    cc_city             Utf8,
    cc_county           Utf8,
    cc_state            Utf8,
    cc_zip              Utf8,
    cc_country          Utf8,
    cc_gmt_offset       Double,
    cc_tax_percentage   Double,
    PRIMARY KEY (cc_call_center_sk)
)
PARTITION BY HASH (cc_call_center_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= store
CREATE TABLE store (
    s_store_sk           Int64 NOT NULL,
    s_store_id           Utf8,
    s_rec_start_date     Utf8,
    s_rec_end_date       Utf8,
    s_closed_date_sk     Int64,
    s_store_name         Utf8,
    s_number_employees   Int64,
    s_floor_space        Int64,
    s_hours              Utf8,
    s_manager            Utf8,
    s_market_id          Int64,
    s_geography_class    Utf8,
    s_market_desc        Utf8,
    s_market_manager     Utf8,
    s_division_id        Int64,
    s_division_name      Utf8,
    s_company_id         Int64,
    s_company_name       Utf8,
    s_street_number      Utf8,
    s_street_name        Utf8,
    s_street_type        Utf8,
    s_suite_number       Utf8,
    s_city               Utf8,
    s_county             Utf8,
    s_state              Utf8,
    s_zip                Utf8,
    s_country            Utf8,
    s_gmt_offset         Double,
    s_tax_precentage     Double,
    PRIMARY KEY (s_store_sk)
)
PARTITION BY HASH (s_store_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= promotion
CREATE TABLE promotion (
    p_promo_sk          Int64 NOT NULL,
    p_promo_id          Utf8,
    p_start_date_sk     Int64,
    p_end_date_sk       Int64,
    p_item_sk           Int64,
    p_cost              Double,
    p_response_target   Int64,
    p_promo_name        Utf8,
    p_channel_dmail     Utf8,
    p_channel_email     Utf8,
    p_channel_catalog   Utf8,
    p_channel_tv        Utf8,
    p_channel_radio     Utf8,
    p_channel_press     Utf8,
    p_channel_event     Utf8,
    p_channel_demo      Utf8,
    p_channel_details   Utf8,
    p_purpose           Utf8,
    p_discount_active   Utf8,
    PRIMARY KEY (p_promo_sk)
)
PARTITION BY HASH (p_promo_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 1
)
--= item
CREATE TABLE item (
    i_item_sk          Int64 NOT NULL,
    i_item_id          Utf8,
    i_rec_start_date   Utf8,
    i_rec_end_date     Utf8,
    i_item_desc        Utf8,
    i_current_price    Double,
    i_wholesale_cost   Double,
    i_brand_id         Int64,
    i_brand            Utf8,
    i_class_id         Int64,
    i_class            Utf8,
    i_category_id      Int64,
    i_category         Utf8,
    i_manufact_id      Int64,
    i_manufact         Utf8,
    i_size             Utf8,
    i_formulation      Utf8,
    i_color            Utf8,
    i_units            Utf8,
    i_container        Utf8,
    i_manager_id       Int64,
    i_product_name     Utf8,
    PRIMARY KEY (i_item_sk)
)
PARTITION BY HASH (i_item_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= inventory
CREATE TABLE inventory (
    inv_date_sk            Int64 NOT NULL,
    inv_item_sk            Int64 NOT NULL,
    inv_warehouse_sk       Int64 NOT NULL,
    inv_quantity_on_hand   Int64,
    PRIMARY KEY (inv_date_sk, inv_item_sk, inv_warehouse_sk)
)
PARTITION BY HASH (inv_date_sk, inv_item_sk, inv_warehouse_sk)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= store_sales
CREATE TABLE store_sales (
    ss_sold_date_sk         Int64,
    ss_sold_time_sk         Int64,
    ss_item_sk              Int64 NOT NULL,
    ss_customer_sk          Int64,
    ss_cdemo_sk             Int64,
    ss_hdemo_sk             Int64,
    ss_addr_sk              Int64,
    ss_store_sk             Int64,
    ss_promo_sk             Int64,
    ss_ticket_number        Int64 NOT NULL,
    ss_quantity             Int64,
    ss_wholesale_cost       Double,
    ss_list_price           Double,
    ss_sales_price          Double,
    ss_ext_discount_amt     Double,
    ss_ext_sales_price      Double,
    ss_ext_wholesale_cost   Double,
    ss_ext_list_price       Double,
    ss_ext_tax              Double,
    ss_coupon_amt           Double,
    ss_net_paid             Double,
    ss_net_paid_inc_tax     Double,
    ss_net_profit           Double,
    PRIMARY KEY (ss_item_sk, ss_ticket_number)
)
PARTITION BY HASH (ss_item_sk, ss_ticket_number)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= store_returns
CREATE TABLE store_returns (
    sr_returned_date_sk     Int64,
    sr_return_time_sk       Int64,
    sr_item_sk              Int64 NOT NULL,
    sr_customer_sk          Int64,
    sr_cdemo_sk             Int64,
    sr_hdemo_sk             Int64,
    sr_addr_sk              Int64,
    sr_store_sk             Int64,
    sr_reason_sk            Int64,
    sr_ticket_number        Int64 NOT NULL,
    sr_return_quantity      Int64,
    sr_return_amt           Double,
    sr_return_tax           Double,
    sr_return_amt_inc_tax   Double,
    sr_fee                  Double,
    sr_return_ship_cost     Double,
    sr_refunded_cash        Double,
    sr_reversed_charge      Double,
    sr_store_credit         Double,
    sr_net_loss             Double,
    PRIMARY KEY (sr_item_sk, sr_ticket_number)
)
PARTITION BY HASH (sr_item_sk, sr_ticket_number)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= catalog_sales
CREATE TABLE catalog_sales (
    cs_sold_date_sk            Int64,
    cs_sold_time_sk            Int64,
    cs_ship_date_sk            Int64,
    cs_bill_customer_sk        Int64,
    cs_bill_cdemo_sk           Int64,
    cs_bill_hdemo_sk           Int64,
    cs_bill_addr_sk            Int64,
    cs_ship_customer_sk        Int64,
    cs_ship_cdemo_sk           Int64,
    cs_ship_hdemo_sk           Int64,
    cs_ship_addr_sk            Int64,
    cs_call_center_sk          Int64,
    cs_catalog_page_sk         Int64,
    cs_ship_mode_sk            Int64,
    cs_warehouse_sk            Int64,
    cs_item_sk                 Int64 NOT NULL,
    cs_promo_sk                Int64,
    cs_order_number            Int64 NOT NULL,
    cs_quantity                Int64,
    cs_wholesale_cost          Double,
    cs_list_price              Double,
    cs_sales_price             Double,
    cs_ext_discount_amt        Double,
    cs_ext_sales_price         Double,
    cs_ext_wholesale_cost      Double,
    cs_ext_list_price          Double,
    cs_ext_tax                 Double,
    cs_coupon_amt              Double,
    cs_ext_ship_cost           Double,
    cs_net_paid                Double,
    cs_net_paid_inc_tax        Double,
    cs_net_paid_inc_ship       Double,
    cs_net_paid_inc_ship_tax   Double,
    cs_net_profit              Double,
    PRIMARY KEY (cs_item_sk, cs_order_number)
)
PARTITION BY HASH (cs_item_sk, cs_order_number)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= catalog_returns
CREATE TABLE catalog_returns (
    cr_returned_date_sk        Int64,
    cr_returned_time_sk        Int64,
    cr_item_sk                 Int64 NOT NULL,
    cr_refunded_customer_sk    Int64,
    cr_refunded_cdemo_sk       Int64,
    cr_refunded_hdemo_sk       Int64,
    cr_refunded_addr_sk        Int64,
    cr_returning_customer_sk   Int64,
    cr_returning_cdemo_sk      Int64,
    cr_returning_hdemo_sk      Int64,
    cr_returning_addr_sk       Int64,
    cr_call_center_sk          Int64,
    cr_catalog_page_sk         Int64,
    cr_ship_mode_sk            Int64,
    cr_warehouse_sk            Int64,
    cr_reason_sk               Int64,
    cr_order_number            Int64 NOT NULL,
    cr_return_quantity         Int64,
    cr_return_amount           Double,
    cr_return_tax              Double,
    cr_return_amt_inc_tax      Double,
    cr_fee                     Double,
    cr_return_ship_cost        Double,
    cr_refunded_cash           Double,
    cr_reversed_charge         Double,
    cr_store_credit            Double,
    cr_net_loss                Double,
    PRIMARY KEY (cr_item_sk, cr_order_number)
)
PARTITION BY HASH (cr_item_sk, cr_order_number)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= web_sales
CREATE TABLE web_sales (
    ws_sold_date_sk            Int64,
    ws_sold_time_sk            Int64,
    ws_ship_date_sk            Int64,
    ws_item_sk                 Int64 NOT NULL,
    ws_bill_customer_sk        Int64,
    ws_bill_cdemo_sk           Int64,
    ws_bill_hdemo_sk           Int64,
    ws_bill_addr_sk            Int64,
    ws_ship_customer_sk        Int64,
    ws_ship_cdemo_sk           Int64,
    ws_ship_hdemo_sk           Int64,
    ws_ship_addr_sk            Int64,
    ws_web_page_sk             Int64,
    ws_web_site_sk             Int64,
    ws_ship_mode_sk            Int64,
    ws_warehouse_sk            Int64,
    ws_promo_sk                Int64,
    ws_order_number            Int64 NOT NULL,
    ws_quantity                Int64,
    ws_wholesale_cost          Double,
    ws_list_price              Double,
    ws_sales_price             Double,
    ws_ext_discount_amt        Double,
    ws_ext_sales_price         Double,
    ws_ext_wholesale_cost      Double,
    ws_ext_list_price          Double,
    ws_ext_tax                 Double,
    ws_coupon_amt              Double,
    ws_ext_ship_cost           Double,
    ws_net_paid                Double,
    ws_net_paid_inc_tax        Double,
    ws_net_paid_inc_ship       Double,
    ws_net_paid_inc_ship_tax   Double,
    ws_net_profit              Double,
    PRIMARY KEY (ws_item_sk, ws_order_number)
)
PARTITION BY HASH (ws_item_sk, ws_order_number)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)
--= web_returns
CREATE TABLE web_returns (
    wr_returned_date_sk        Int64,
    wr_returned_time_sk        Int64,
    wr_item_sk                 Int64 NOT NULL,
    wr_refunded_customer_sk    Int64,
    wr_refunded_cdemo_sk       Int64,
    wr_refunded_hdemo_sk       Int64,
    wr_refunded_addr_sk        Int64,
    wr_returning_customer_sk   Int64,
    wr_returning_cdemo_sk      Int64,
    wr_returning_hdemo_sk      Int64,
    wr_returning_addr_sk       Int64,
    wr_web_page_sk             Int64,
    wr_reason_sk               Int64,
    wr_order_number            Int64 NOT NULL,
    wr_return_quantity         Int64,
    wr_return_amt              Double,
    wr_return_tax              Double,
    wr_return_amt_inc_tax      Double,
    wr_fee                     Double,
    wr_return_ship_cost        Double,
    wr_refunded_cash           Double,
    wr_reversed_charge         Double,
    wr_account_credit          Double,
    wr_net_loss                Double,
    PRIMARY KEY (wr_item_sk, wr_order_number)
)
PARTITION BY HASH (wr_item_sk, wr_order_number)
WITH (
    STORE = COLUMN,
    AUTO_PARTITIONING_MIN_PARTITIONS_COUNT = 64
)

--+ create_indexes
--= noop
SELECT 1
