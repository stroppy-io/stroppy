-- TPC-DS schema for picodata (sbroad / Tarantool). Column names and order
-- match the ported dsdgen emission and the pg schema; types are adapted to
-- sbroad's SQL subset (probed against picodata 26.3):
--   - char(N) -> varchar(N): sbroad has no fixed-width CHAR.
--   - date    -> datetime:   sbroad has no DATE type (Tarantool datetime).
--   - every table carries a PRIMARY KEY (Tarantool spaces require one);
--     key columns are NOT NULL. No FOREIGN KEY (sbroad limitation).
-- Section layout mirrors schema.pg.sql so tpcds.ts needs no schema-shape
-- branch: drop_schema (plain DROP TABLE IF EXISTS, no CASCADE), create_schema,
-- create_indexes. set_timeout/preconfigure_db are omitted (pg-specific SETs;
-- absent sections are no-ops in the dump pass).
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
--= create_income_band
CREATE TABLE income_band (

    ib_income_band_sk          integer NOT NULL,
    ib_lower_bound             integer,
    ib_upper_bound             integer,
    PRIMARY KEY (ib_income_band_sk)
)
--= create_ship_mode
CREATE TABLE ship_mode (

    sm_ship_mode_sk            integer NOT NULL,
    sm_ship_mode_id            varchar(16),
    sm_type                    varchar(30),
    sm_code                    varchar(10),
    sm_carrier                 varchar(20),
    sm_contract                varchar(20),
    PRIMARY KEY (sm_ship_mode_sk)
)
--= create_reason
CREATE TABLE reason (

    r_reason_sk                integer NOT NULL,
    r_reason_id                varchar(16),
    r_reason_desc              varchar(100),
    PRIMARY KEY (r_reason_sk)
)
--= create_household_demographics
CREATE TABLE household_demographics (

    hd_demo_sk                 integer NOT NULL,
    hd_income_band_sk          integer,
    hd_buy_potential           varchar(15),
    hd_dep_count               integer,
    hd_vehicle_count           integer,
    PRIMARY KEY (hd_demo_sk)
)
--= create_customer_demographics
CREATE TABLE customer_demographics (

    cd_demo_sk                 integer NOT NULL,
    cd_gender                  varchar(1),
    cd_marital_status          varchar(1),
    cd_education_status        varchar(20),
    cd_purchase_estimate       integer,
    cd_credit_rating           varchar(10),
    cd_dep_count               integer,
    cd_dep_employed_count      integer,
    cd_dep_college_count       integer,
    PRIMARY KEY (cd_demo_sk)
)
--= create_date_dim
CREATE TABLE date_dim (

    d_date_sk                  integer NOT NULL,
    d_date_id                  varchar(16),
    d_date                     datetime,
    d_month_seq                integer,
    d_week_seq                 integer,
    d_quarter_seq              integer,
    d_year                     integer,
    d_dow                      integer,
    d_moy                      integer,
    d_dom                      integer,
    d_qoy                      integer,
    d_fy_year                  integer,
    d_fy_quarter_seq           integer,
    d_fy_week_seq              integer,
    d_day_name                 varchar(9),
    d_quarter_name             varchar(6),
    d_holiday                  varchar(1),
    d_weekend                  varchar(1),
    d_following_holiday        varchar(1),
    d_first_dom                integer,
    d_last_dom                 integer,
    d_same_day_ly              integer,
    d_same_day_lq              integer,
    d_current_day              varchar(1),
    d_current_week             varchar(1),
    d_current_month            varchar(1),
    d_current_quarter          varchar(1),
    d_current_year             varchar(1),
    PRIMARY KEY (d_date_sk)
)
--= create_time_dim
CREATE TABLE time_dim (

    t_time_sk                  integer NOT NULL,
    t_time_id                  varchar(16),
    t_time                     integer,
    t_hour                     integer,
    t_minute                   integer,
    t_second                   integer,
    t_am_pm                    varchar(2),
    t_shift                    varchar(20),
    t_sub_shift                varchar(20),
    t_meal_time                varchar(20),
    PRIMARY KEY (t_time_sk)
)
--= create_warehouse
CREATE TABLE warehouse (

    w_warehouse_sk             integer NOT NULL,
    w_warehouse_id             varchar(16),
    w_warehouse_name           varchar(20),
    w_warehouse_sq_ft          integer,
    w_street_number            varchar(10),
    w_street_name              varchar(60),
    w_street_type              varchar(15),
    w_suite_number             varchar(10),
    w_city                     varchar(60),
    w_county                   varchar(30),
    w_state                    varchar(2),
    w_zip                      varchar(10),
    w_country                  varchar(20),
    w_gmt_offset               decimal(5,2),
    PRIMARY KEY (w_warehouse_sk)
)
--= create_web_page
CREATE TABLE web_page (

    wp_web_page_sk             integer NOT NULL,
    wp_web_page_id             varchar(16),
    wp_rec_start_date          datetime,
    wp_rec_end_date            datetime,
    wp_creation_date_sk        integer,
    wp_access_date_sk          integer,
    wp_autogen_flag            varchar(1),
    wp_customer_sk             integer,
    wp_url                     varchar(100),
    wp_type                    varchar(50),
    wp_char_count              integer,
    wp_link_count              integer,
    wp_image_count             integer,
    wp_max_ad_count            integer,
    PRIMARY KEY (wp_web_page_sk)
)
--= create_web_site
CREATE TABLE web_site (

    web_site_sk                integer NOT NULL,
    web_site_id                varchar(16),
    web_rec_start_date         datetime,
    web_rec_end_date           datetime,
    web_name                   varchar(50),
    web_open_date_sk           integer,
    web_close_date_sk          integer,
    web_class                  varchar(50),
    web_manager                varchar(40),
    web_mkt_id                 integer,
    web_mkt_class              varchar(50),
    web_mkt_desc               varchar(100),
    web_market_manager         varchar(40),
    web_company_id             integer,
    web_company_name           varchar(50),
    web_street_number          varchar(10),
    web_street_name            varchar(60),
    web_street_type            varchar(15),
    web_suite_number           varchar(10),
    web_city                   varchar(60),
    web_county                 varchar(30),
    web_state                  varchar(2),
    web_zip                    varchar(10),
    web_country                varchar(20),
    web_gmt_offset             decimal(5,2),
    web_tax_percentage         decimal(5,2),
    PRIMARY KEY (web_site_sk)
)
--= create_catalog_page
CREATE TABLE catalog_page (

    cp_catalog_page_sk         integer NOT NULL,
    cp_catalog_page_id         varchar(16),
    cp_start_date_sk           integer,
    cp_end_date_sk             integer,
    cp_department              varchar(50),
    cp_catalog_number          integer,
    cp_catalog_page_number     integer,
    cp_description             varchar(100),
    cp_type                    varchar(100),
    PRIMARY KEY (cp_catalog_page_sk)
)
--= create_customer_address
CREATE TABLE customer_address (

    ca_address_sk              integer NOT NULL,
    ca_address_id              varchar(16),
    ca_street_number           varchar(10),
    ca_street_name             varchar(60),
    ca_street_type             varchar(15),
    ca_suite_number            varchar(10),
    ca_city                    varchar(60),
    ca_county                  varchar(30),
    ca_state                   varchar(2),
    ca_zip                     varchar(10),
    ca_country                 varchar(20),
    ca_gmt_offset              decimal(5,2),
    ca_location_type           varchar(20),
    PRIMARY KEY (ca_address_sk)
)
--= create_customer
CREATE TABLE customer (

    c_customer_sk              integer NOT NULL,
    c_customer_id              varchar(16),
    c_current_cdemo_sk         integer,
    c_current_hdemo_sk         integer,
    c_current_addr_sk          integer,
    c_first_shipto_date_sk     integer,
    c_first_sales_date_sk      integer,
    c_salutation               varchar(10),
    c_first_name               varchar(20),
    c_last_name                varchar(30),
    c_preferred_cust_flag      varchar(1),
    c_birth_day                integer,
    c_birth_month              integer,
    c_birth_year               integer,
    c_birth_country            varchar(20),
    c_login                    varchar(13),
    c_email_address            varchar(50),
    c_last_review_date         varchar(10),
    PRIMARY KEY (c_customer_sk)
)
--= create_call_center
CREATE TABLE call_center (

    cc_call_center_sk          integer NOT NULL,
    cc_call_center_id          varchar(16),
    cc_rec_start_date          datetime,
    cc_rec_end_date            datetime,
    cc_closed_date_sk          integer,
    cc_open_date_sk            integer,
    cc_name                    varchar(50),
    cc_class                   varchar(50),
    cc_employees               integer,
    cc_sq_ft                   integer,
    cc_hours                   varchar(20),
    cc_manager                 varchar(40),
    cc_mkt_id                  integer,
    cc_mkt_class               varchar(50),
    cc_mkt_desc                varchar(100),
    cc_market_manager          varchar(40),
    cc_division                integer,
    cc_division_name           varchar(50),
    cc_company                 integer,
    cc_company_name            varchar(50),
    cc_street_number           varchar(10),
    cc_street_name             varchar(60),
    cc_street_type             varchar(15),
    cc_suite_number            varchar(10),
    cc_city                    varchar(60),
    cc_county                  varchar(30),
    cc_state                   varchar(2),
    cc_zip                     varchar(10),
    cc_country                 varchar(20),
    cc_gmt_offset              decimal(5,2),
    cc_tax_percentage          decimal(5,2),
    PRIMARY KEY (cc_call_center_sk)
)
--= create_store
CREATE TABLE store (

    s_store_sk                 integer NOT NULL,
    s_store_id                 varchar(16),
    s_rec_start_date           datetime,
    s_rec_end_date             datetime,
    s_closed_date_sk           integer,
    s_store_name               varchar(50),
    s_number_employees         integer,
    s_floor_space              integer,
    s_hours                    varchar(20),
    s_manager                  varchar(40),
    s_market_id                integer,
    s_geography_class          varchar(100),
    s_market_desc              varchar(100),
    s_market_manager           varchar(40),
    s_division_id              integer,
    s_division_name            varchar(50),
    s_company_id               integer,
    s_company_name             varchar(50),
    s_street_number            varchar(10),
    s_street_name              varchar(60),
    s_street_type              varchar(15),
    s_suite_number             varchar(10),
    s_city                     varchar(60),
    s_county                   varchar(30),
    s_state                    varchar(2),
    s_zip                      varchar(10),
    s_country                  varchar(20),
    s_gmt_offset               decimal(5,2),
    s_tax_precentage           decimal(5,2),
    PRIMARY KEY (s_store_sk)
)
--= create_promotion
CREATE TABLE promotion (

    p_promo_sk                 integer NOT NULL,
    p_promo_id                 varchar(16),
    p_start_date_sk            integer,
    p_end_date_sk              integer,
    p_item_sk                  integer,
    p_cost                     decimal(15,2),
    p_response_target          integer,
    p_promo_name               varchar(50),
    p_channel_dmail            varchar(1),
    p_channel_email            varchar(1),
    p_channel_catalog          varchar(1),
    p_channel_tv               varchar(1),
    p_channel_radio            varchar(1),
    p_channel_press            varchar(1),
    p_channel_event            varchar(1),
    p_channel_demo             varchar(1),
    p_channel_details          varchar(100),
    p_purpose                  varchar(15),
    p_discount_active          varchar(1),
    PRIMARY KEY (p_promo_sk)
)
--= create_item
CREATE TABLE item (

    i_item_sk                  integer NOT NULL,
    i_item_id                  varchar(16),
    i_rec_start_date           datetime,
    i_rec_end_date             datetime,
    i_item_desc                varchar(200),
    i_current_price            decimal(7,2),
    i_wholesale_cost           decimal(7,2),
    i_brand_id                 integer,
    i_brand                    varchar(50),
    i_class_id                 integer,
    i_class                    varchar(50),
    i_category_id              integer,
    i_category                 varchar(50),
    i_manufact_id              integer,
    i_manufact                 varchar(50),
    i_size                     varchar(20),
    i_formulation              varchar(20),
    i_color                    varchar(20),
    i_units                    varchar(10),
    i_container                varchar(10),
    i_manager_id               integer,
    i_product_name             varchar(50),
    PRIMARY KEY (i_item_sk)
)
--= create_inventory
CREATE TABLE inventory (

    inv_date_sk                integer NOT NULL,
    inv_item_sk                integer NOT NULL,
    inv_warehouse_sk           integer NOT NULL,
    inv_quantity_on_hand       integer,
    PRIMARY KEY (inv_date_sk, inv_item_sk, inv_warehouse_sk)
)
--= create_store_sales
CREATE TABLE store_sales (

    ss_sold_date_sk            integer,
    ss_sold_time_sk            integer,
    ss_item_sk                 integer NOT NULL,
    ss_customer_sk             integer,
    ss_cdemo_sk                integer,
    ss_hdemo_sk                integer,
    ss_addr_sk                 integer,
    ss_store_sk                integer,
    ss_promo_sk                integer,
    ss_ticket_number           integer NOT NULL,
    ss_quantity                integer,
    ss_wholesale_cost          decimal(7,2),
    ss_list_price              decimal(7,2),
    ss_sales_price             decimal(7,2),
    ss_ext_discount_amt        decimal(7,2),
    ss_ext_sales_price         decimal(7,2),
    ss_ext_wholesale_cost      decimal(7,2),
    ss_ext_list_price          decimal(7,2),
    ss_ext_tax                 decimal(7,2),
    ss_coupon_amt              decimal(7,2),
    ss_net_paid                decimal(7,2),
    ss_net_paid_inc_tax        decimal(7,2),
    ss_net_profit              decimal(7,2),
    PRIMARY KEY (ss_item_sk, ss_ticket_number)
)
--= create_store_returns
CREATE TABLE store_returns (

    sr_returned_date_sk        integer,
    sr_return_time_sk          integer,
    sr_item_sk                 integer NOT NULL,
    sr_customer_sk             integer,
    sr_cdemo_sk                integer,
    sr_hdemo_sk                integer,
    sr_addr_sk                 integer,
    sr_store_sk                integer,
    sr_reason_sk               integer,
    sr_ticket_number           integer NOT NULL,
    sr_return_quantity         integer,
    sr_return_amt              decimal(7,2),
    sr_return_tax              decimal(7,2),
    sr_return_amt_inc_tax      decimal(7,2),
    sr_fee                     decimal(7,2),
    sr_return_ship_cost        decimal(7,2),
    sr_refunded_cash           decimal(7,2),
    sr_reversed_charge         decimal(7,2),
    sr_store_credit            decimal(7,2),
    sr_net_loss                decimal(7,2),
    PRIMARY KEY (sr_item_sk, sr_ticket_number)
)
--= create_catalog_sales
CREATE TABLE catalog_sales (

    cs_sold_date_sk            integer,
    cs_sold_time_sk            integer,
    cs_ship_date_sk            integer,
    cs_bill_customer_sk        integer,
    cs_bill_cdemo_sk           integer,
    cs_bill_hdemo_sk           integer,
    cs_bill_addr_sk            integer,
    cs_ship_customer_sk        integer,
    cs_ship_cdemo_sk           integer,
    cs_ship_hdemo_sk           integer,
    cs_ship_addr_sk            integer,
    cs_call_center_sk          integer,
    cs_catalog_page_sk         integer,
    cs_ship_mode_sk            integer,
    cs_warehouse_sk            integer,
    cs_item_sk                 integer NOT NULL,
    cs_promo_sk                integer,
    cs_order_number            integer NOT NULL,
    cs_quantity                integer,
    cs_wholesale_cost          decimal(7,2),
    cs_list_price              decimal(7,2),
    cs_sales_price             decimal(7,2),
    cs_ext_discount_amt        decimal(7,2),
    cs_ext_sales_price         decimal(7,2),
    cs_ext_wholesale_cost      decimal(7,2),
    cs_ext_list_price          decimal(7,2),
    cs_ext_tax                 decimal(7,2),
    cs_coupon_amt              decimal(7,2),
    cs_ext_ship_cost           decimal(7,2),
    cs_net_paid                decimal(7,2),
    cs_net_paid_inc_tax        decimal(7,2),
    cs_net_paid_inc_ship       decimal(7,2),
    cs_net_paid_inc_ship_tax   decimal(7,2),
    cs_net_profit              decimal(7,2),
    PRIMARY KEY (cs_item_sk, cs_order_number)
)
--= create_catalog_returns
CREATE TABLE catalog_returns (

    cr_returned_date_sk        integer,
    cr_returned_time_sk        integer,
    cr_item_sk                 integer NOT NULL,
    cr_refunded_customer_sk    integer,
    cr_refunded_cdemo_sk       integer,
    cr_refunded_hdemo_sk       integer,
    cr_refunded_addr_sk        integer,
    cr_returning_customer_sk   integer,
    cr_returning_cdemo_sk      integer,
    cr_returning_hdemo_sk      integer,
    cr_returning_addr_sk       integer,
    cr_call_center_sk          integer,
    cr_catalog_page_sk         integer,
    cr_ship_mode_sk            integer,
    cr_warehouse_sk            integer,
    cr_reason_sk               integer,
    cr_order_number            integer NOT NULL,
    cr_return_quantity         integer,
    cr_return_amount           decimal(7,2),
    cr_return_tax              decimal(7,2),
    cr_return_amt_inc_tax      decimal(7,2),
    cr_fee                     decimal(7,2),
    cr_return_ship_cost        decimal(7,2),
    cr_refunded_cash           decimal(7,2),
    cr_reversed_charge         decimal(7,2),
    cr_store_credit            decimal(7,2),
    cr_net_loss                decimal(7,2),
    PRIMARY KEY (cr_item_sk, cr_order_number)
)
--= create_web_sales
CREATE TABLE web_sales (

    ws_sold_date_sk            integer,
    ws_sold_time_sk            integer,
    ws_ship_date_sk            integer,
    ws_item_sk                 integer NOT NULL,
    ws_bill_customer_sk        integer,
    ws_bill_cdemo_sk           integer,
    ws_bill_hdemo_sk           integer,
    ws_bill_addr_sk            integer,
    ws_ship_customer_sk        integer,
    ws_ship_cdemo_sk           integer,
    ws_ship_hdemo_sk           integer,
    ws_ship_addr_sk            integer,
    ws_web_page_sk             integer,
    ws_web_site_sk             integer,
    ws_ship_mode_sk            integer,
    ws_warehouse_sk            integer,
    ws_promo_sk                integer,
    ws_order_number            integer NOT NULL,
    ws_quantity                integer,
    ws_wholesale_cost          decimal(7,2),
    ws_list_price              decimal(7,2),
    ws_sales_price             decimal(7,2),
    ws_ext_discount_amt        decimal(7,2),
    ws_ext_sales_price         decimal(7,2),
    ws_ext_wholesale_cost      decimal(7,2),
    ws_ext_list_price          decimal(7,2),
    ws_ext_tax                 decimal(7,2),
    ws_coupon_amt              decimal(7,2),
    ws_ext_ship_cost           decimal(7,2),
    ws_net_paid                decimal(7,2),
    ws_net_paid_inc_tax        decimal(7,2),
    ws_net_paid_inc_ship       decimal(7,2),
    ws_net_paid_inc_ship_tax   decimal(7,2),
    ws_net_profit              decimal(7,2),
    PRIMARY KEY (ws_item_sk, ws_order_number)
)
--= create_web_returns
CREATE TABLE web_returns (

    wr_returned_date_sk        integer,
    wr_returned_time_sk        integer,
    wr_item_sk                 integer NOT NULL,
    wr_refunded_customer_sk    integer,
    wr_refunded_cdemo_sk       integer,
    wr_refunded_hdemo_sk       integer,
    wr_refunded_addr_sk        integer,
    wr_returning_customer_sk   integer,
    wr_returning_cdemo_sk      integer,
    wr_returning_hdemo_sk      integer,
    wr_returning_addr_sk       integer,
    wr_web_page_sk             integer,
    wr_reason_sk               integer,
    wr_order_number            integer NOT NULL,
    wr_return_quantity         integer,
    wr_return_amt              decimal(7,2),
    wr_return_tax              decimal(7,2),
    wr_return_amt_inc_tax      decimal(7,2),
    wr_fee                     decimal(7,2),
    wr_return_ship_cost        decimal(7,2),
    wr_refunded_cash           decimal(7,2),
    wr_reversed_charge         decimal(7,2),
    wr_account_credit          decimal(7,2),
    wr_net_loss                decimal(7,2),
    PRIMARY KEY (wr_item_sk, wr_order_number)
)

--+ create_indexes
-- dimension keys (fact tables join to these)
--= i_d_date_sk
create index i_d_date_sk on date_dim (d_date_sk);
--= i_d_year
create index i_d_year on date_dim (d_year);
--= i_d_month_seq
create index i_d_month_seq on date_dim (d_month_seq);
--= i_d_week_seq
create index i_d_week_seq on date_dim (d_week_seq);
--= i_t_time_sk
create index i_t_time_sk on time_dim (t_time_sk);
--= i_i_item_sk
create index i_i_item_sk on item (i_item_sk);
--= i_i_category
create index i_i_category on item (i_category);
--= i_c_customer_sk
create index i_c_customer_sk on customer (c_customer_sk);
--= i_c_current_addr_sk
create index i_c_current_addr_sk on customer (c_current_addr_sk);
--= i_c_current_cdemo_sk
create index i_c_current_cdemo_sk on customer (c_current_cdemo_sk);
--= i_c_current_hdemo_sk
create index i_c_current_hdemo_sk on customer (c_current_hdemo_sk);
--= i_ca_address_sk
create index i_ca_address_sk on customer_address (ca_address_sk);
--= i_cd_demo_sk
create index i_cd_demo_sk on customer_demographics (cd_demo_sk);
--= i_hd_demo_sk
create index i_hd_demo_sk on household_demographics (hd_demo_sk);
--= i_hd_income_band_sk
create index i_hd_income_band_sk on household_demographics (hd_income_band_sk);
--= i_ib_income_band_sk
create index i_ib_income_band_sk on income_band (ib_income_band_sk);
--= i_s_store_sk
create index i_s_store_sk on store (s_store_sk);
--= i_w_warehouse_sk
create index i_w_warehouse_sk on warehouse (w_warehouse_sk);
--= i_p_promo_sk
create index i_p_promo_sk on promotion (p_promo_sk);
--= i_cc_call_center_sk
create index i_cc_call_center_sk on call_center (cc_call_center_sk);
--= i_cp_catalog_page_sk
create index i_cp_catalog_page_sk on catalog_page (cp_catalog_page_sk);
--= i_wp_web_page_sk
create index i_wp_web_page_sk on web_page (wp_web_page_sk);
--= i_web_site_sk
create index i_web_site_sk on web_site (web_site_sk);
--= i_sm_ship_mode_sk
create index i_sm_ship_mode_sk on ship_mode (sm_ship_mode_sk);
--= i_r_reason_sk
create index i_r_reason_sk on reason (r_reason_sk);
-- store_sales / store_returns
--= i_ss_sold_date_sk
create index i_ss_sold_date_sk on store_sales (ss_sold_date_sk);
--= i_ss_item_sk
create index i_ss_item_sk on store_sales (ss_item_sk);
--= i_ss_customer_sk
create index i_ss_customer_sk on store_sales (ss_customer_sk);
--= i_ss_cdemo_sk
create index i_ss_cdemo_sk on store_sales (ss_cdemo_sk);
--= i_ss_hdemo_sk
create index i_ss_hdemo_sk on store_sales (ss_hdemo_sk);
--= i_ss_addr_sk
create index i_ss_addr_sk on store_sales (ss_addr_sk);
--= i_ss_store_sk
create index i_ss_store_sk on store_sales (ss_store_sk);
--= i_ss_promo_sk
create index i_ss_promo_sk on store_sales (ss_promo_sk);
--= i_ss_ticket_number
create index i_ss_ticket_number on store_sales (ss_ticket_number);
--= i_sr_returned_date_sk
create index i_sr_returned_date_sk on store_returns (sr_returned_date_sk);
--= i_sr_item_sk
create index i_sr_item_sk on store_returns (sr_item_sk);
--= i_sr_customer_sk
create index i_sr_customer_sk on store_returns (sr_customer_sk);
--= i_sr_ticket_number
create index i_sr_ticket_number on store_returns (sr_ticket_number);
-- catalog_sales / catalog_returns
--= i_cs_sold_date_sk
create index i_cs_sold_date_sk on catalog_sales (cs_sold_date_sk);
--= i_cs_ship_date_sk
create index i_cs_ship_date_sk on catalog_sales (cs_ship_date_sk);
--= i_cs_item_sk
create index i_cs_item_sk on catalog_sales (cs_item_sk);
--= i_cs_bill_customer_sk
create index i_cs_bill_customer_sk on catalog_sales (cs_bill_customer_sk);
--= i_cs_bill_cdemo_sk
create index i_cs_bill_cdemo_sk on catalog_sales (cs_bill_cdemo_sk);
--= i_cs_bill_hdemo_sk
create index i_cs_bill_hdemo_sk on catalog_sales (cs_bill_hdemo_sk);
--= i_cs_bill_addr_sk
create index i_cs_bill_addr_sk on catalog_sales (cs_bill_addr_sk);
--= i_cs_warehouse_sk
create index i_cs_warehouse_sk on catalog_sales (cs_warehouse_sk);
--= i_cs_order_number
create index i_cs_order_number on catalog_sales (cs_order_number);
--= i_cr_returned_date_sk
create index i_cr_returned_date_sk on catalog_returns (cr_returned_date_sk);
--= i_cr_item_sk
create index i_cr_item_sk on catalog_returns (cr_item_sk);
--= i_cr_order_number
create index i_cr_order_number on catalog_returns (cr_order_number);
--= i_cr_returning_customer_sk
create index i_cr_returning_customer_sk on catalog_returns (cr_returning_customer_sk);
--= i_cr_returning_addr_sk
create index i_cr_returning_addr_sk on catalog_returns (cr_returning_addr_sk);
-- web_sales / web_returns
--= i_ws_sold_date_sk
create index i_ws_sold_date_sk on web_sales (ws_sold_date_sk);
--= i_ws_ship_date_sk
create index i_ws_ship_date_sk on web_sales (ws_ship_date_sk);
--= i_ws_item_sk
create index i_ws_item_sk on web_sales (ws_item_sk);
--= i_ws_bill_customer_sk
create index i_ws_bill_customer_sk on web_sales (ws_bill_customer_sk);
--= i_ws_order_number
create index i_ws_order_number on web_sales (ws_order_number);
--= i_wr_returned_date_sk
create index i_wr_returned_date_sk on web_returns (wr_returned_date_sk);
--= i_wr_item_sk
create index i_wr_item_sk on web_returns (wr_item_sk);
--= i_wr_order_number
create index i_wr_order_number on web_returns (wr_order_number);
-- inventory
--= i_inv_date_sk
create index i_inv_date_sk on inventory (inv_date_sk);
--= i_inv_item_sk
create index i_inv_item_sk on inventory (inv_item_sk);
--= i_inv_warehouse_sk
create index i_inv_warehouse_sk on inventory (inv_warehouse_sk);
