-- TPC-DS schema (PostgreSQL). Column names match the ported dsdgen
-- generator's emission order; types are the canonical TPC-DS types from the
-- official tpcds.sql. No constraints: the load path uses bulk COPY and the
-- query workload is read-only. One '--+ create_schema' section; each DROP and
-- CREATE is a named '--=' statement, parsed by parse_sql_with_sections like
-- the TPC-H schema.
--+ create_schema
--= drop_income_band
DROP TABLE IF EXISTS income_band;
--= income_band
CREATE TABLE income_band (
    ib_income_band_sk          integer,
    ib_lower_bound             integer,
    ib_upper_bound             integer
);
--= drop_ship_mode
DROP TABLE IF EXISTS ship_mode;
--= ship_mode
CREATE TABLE ship_mode (
    sm_ship_mode_sk            integer,
    sm_ship_mode_id            char(16),
    sm_type                    char(30),
    sm_code                    char(10),
    sm_carrier                 char(20),
    sm_contract                char(20)
);
--= drop_reason
DROP TABLE IF EXISTS reason;
--= reason
CREATE TABLE reason (
    r_reason_sk                integer,
    r_reason_id                char(16),
    r_reason_desc              char(100)
);
--= drop_household_demographics
DROP TABLE IF EXISTS household_demographics;
--= household_demographics
CREATE TABLE household_demographics (
    hd_demo_sk                 integer,
    hd_income_band_sk          integer,
    hd_buy_potential           char(15),
    hd_dep_count               integer,
    hd_vehicle_count           integer
);
--= drop_customer_demographics
DROP TABLE IF EXISTS customer_demographics;
--= customer_demographics
CREATE TABLE customer_demographics (
    cd_demo_sk                 integer,
    cd_gender                  char(1),
    cd_marital_status          char(1),
    cd_education_status        char(20),
    cd_purchase_estimate       integer,
    cd_credit_rating           char(10),
    cd_dep_count               integer,
    cd_dep_employed_count      integer,
    cd_dep_college_count       integer
);
--= drop_date_dim
DROP TABLE IF EXISTS date_dim;
--= date_dim
CREATE TABLE date_dim (
    d_date_sk                  integer,
    d_date_id                  char(16),
    d_date                     date,
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
    d_day_name                 char(9),
    d_quarter_name             char(6),
    d_holiday                  char(1),
    d_weekend                  char(1),
    d_following_holiday        char(1),
    d_first_dom                integer,
    d_last_dom                 integer,
    d_same_day_ly              integer,
    d_same_day_lq              integer,
    d_current_day              char(1),
    d_current_week             char(1),
    d_current_month            char(1),
    d_current_quarter          char(1),
    d_current_year             char(1)
);
--= drop_time_dim
DROP TABLE IF EXISTS time_dim;
--= time_dim
CREATE TABLE time_dim (
    t_time_sk                  integer,
    t_time_id                  char(16),
    t_time                     integer,
    t_hour                     integer,
    t_minute                   integer,
    t_second                   integer,
    t_am_pm                    char(2),
    t_shift                    char(20),
    t_sub_shift                char(20),
    t_meal_time                char(20)
);
--= drop_warehouse
DROP TABLE IF EXISTS warehouse;
--= warehouse
CREATE TABLE warehouse (
    w_warehouse_sk             integer,
    w_warehouse_id             char(16),
    w_warehouse_name           varchar(20),
    w_warehouse_sq_ft          integer,
    w_street_number            char(10),
    w_street_name              varchar(60),
    w_street_type              char(15),
    w_suite_number             char(10),
    w_city                     varchar(60),
    w_county                   varchar(30),
    w_state                    char(2),
    w_zip                      char(10),
    w_country                  varchar(20),
    w_gmt_offset               decimal(5,2)
);
--= drop_web_page
DROP TABLE IF EXISTS web_page;
--= web_page
CREATE TABLE web_page (
    wp_web_page_sk             integer,
    wp_web_page_id             char(16),
    wp_rec_start_date          date,
    wp_rec_end_date            date,
    wp_creation_date_sk        integer,
    wp_access_date_sk          integer,
    wp_autogen_flag            char(1),
    wp_customer_sk             integer,
    wp_url                     varchar(100),
    wp_type                    char(50),
    wp_char_count              integer,
    wp_link_count              integer,
    wp_image_count             integer,
    wp_max_ad_count            integer
);
--= drop_web_site
DROP TABLE IF EXISTS web_site;
--= web_site
CREATE TABLE web_site (
    web_site_sk                integer,
    web_site_id                char(16),
    web_rec_start_date         date,
    web_rec_end_date           date,
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
    web_company_name           char(50),
    web_street_number          char(10),
    web_street_name            varchar(60),
    web_street_type            char(15),
    web_suite_number           char(10),
    web_city                   varchar(60),
    web_county                 varchar(30),
    web_state                  char(2),
    web_zip                    char(10),
    web_country                varchar(20),
    web_gmt_offset             decimal(5,2),
    web_tax_percentage         decimal(5,2)
);
--= drop_catalog_page
DROP TABLE IF EXISTS catalog_page;
--= catalog_page
CREATE TABLE catalog_page (
    cp_catalog_page_sk         integer,
    cp_catalog_page_id         char(16),
    cp_start_date_sk           integer,
    cp_end_date_sk             integer,
    cp_department              varchar(50),
    cp_catalog_number          integer,
    cp_catalog_page_number     integer,
    cp_description             varchar(100),
    cp_type                    varchar(100)
);
--= drop_customer_address
DROP TABLE IF EXISTS customer_address;
--= customer_address
CREATE TABLE customer_address (
    ca_address_sk              integer,
    ca_address_id              char(16),
    ca_street_number           char(10),
    ca_street_name             varchar(60),
    ca_street_type             char(15),
    ca_suite_number            char(10),
    ca_city                    varchar(60),
    ca_county                  varchar(30),
    ca_state                   char(2),
    ca_zip                     char(10),
    ca_country                 varchar(20),
    ca_gmt_offset              decimal(5,2),
    ca_location_type           char(20)
);
--= drop_customer
DROP TABLE IF EXISTS customer;
--= customer
CREATE TABLE customer (
    c_customer_sk              integer,
    c_customer_id              char(16),
    c_current_cdemo_sk         integer,
    c_current_hdemo_sk         integer,
    c_current_addr_sk          integer,
    c_first_shipto_date_sk     integer,
    c_first_sales_date_sk      integer,
    c_salutation               char(10),
    c_first_name               char(20),
    c_last_name                char(30),
    c_preferred_cust_flag      char(1),
    c_birth_day                integer,
    c_birth_month              integer,
    c_birth_year               integer,
    c_birth_country            varchar(20),
    c_login                    char(13),
    c_email_address            char(50),
    c_last_review_date         char(10)
);
--= drop_call_center
DROP TABLE IF EXISTS call_center;
--= call_center
CREATE TABLE call_center (
    cc_call_center_sk          integer,
    cc_call_center_id          char(16),
    cc_rec_start_date          date,
    cc_rec_end_date            date,
    cc_closed_date_sk          integer,
    cc_open_date_sk            integer,
    cc_name                    varchar(50),
    cc_class                   varchar(50),
    cc_employees               integer,
    cc_sq_ft                   integer,
    cc_hours                   char(20),
    cc_manager                 varchar(40),
    cc_mkt_id                  integer,
    cc_mkt_class               char(50),
    cc_mkt_desc                varchar(100),
    cc_market_manager          varchar(40),
    cc_division                integer,
    cc_division_name           varchar(50),
    cc_company                 integer,
    cc_company_name            char(50),
    cc_street_number           char(10),
    cc_street_name             varchar(60),
    cc_street_type             char(15),
    cc_suite_number            char(10),
    cc_city                    varchar(60),
    cc_county                  varchar(30),
    cc_state                   char(2),
    cc_zip                     char(10),
    cc_country                 varchar(20),
    cc_gmt_offset              decimal(5,2),
    cc_tax_percentage          decimal(5,2)
);
--= drop_store
DROP TABLE IF EXISTS store;
--= store
CREATE TABLE store (
    s_store_sk                 integer,
    s_store_id                 char(16),
    s_rec_start_date           date,
    s_rec_end_date             date,
    s_closed_date_sk           integer,
    s_store_name               varchar(50),
    s_number_employees         integer,
    s_floor_space              integer,
    s_hours                    char(20),
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
    s_street_type              char(15),
    s_suite_number             char(10),
    s_city                     varchar(60),
    s_county                   varchar(30),
    s_state                    char(2),
    s_zip                      char(10),
    s_country                  varchar(20),
    s_gmt_offset               decimal(5,2),
    s_tax_precentage           decimal(5,2)
);
--= drop_promotion
DROP TABLE IF EXISTS promotion;
--= promotion
CREATE TABLE promotion (
    p_promo_sk                 integer,
    p_promo_id                 char(16),
    p_start_date_sk            integer,
    p_end_date_sk              integer,
    p_item_sk                  integer,
    p_cost                     decimal(15,2),
    p_response_target          integer,
    p_promo_name               char(50),
    p_channel_dmail            char(1),
    p_channel_email            char(1),
    p_channel_catalog          char(1),
    p_channel_tv               char(1),
    p_channel_radio            char(1),
    p_channel_press            char(1),
    p_channel_event            char(1),
    p_channel_demo             char(1),
    p_channel_details          varchar(100),
    p_purpose                  char(15),
    p_discount_active          char(1)
);
--= drop_item
DROP TABLE IF EXISTS item;
--= item
CREATE TABLE item (
    i_item_sk                  integer,
    i_item_id                  char(16),
    i_rec_start_date           date,
    i_rec_end_date             date,
    i_item_desc                varchar(200),
    i_current_price            decimal(7,2),
    i_wholesale_cost           decimal(7,2),
    i_brand_id                 integer,
    i_brand                    char(50),
    i_class_id                 integer,
    i_class                    char(50),
    i_category_id              integer,
    i_category                 char(50),
    i_manufact_id              integer,
    i_manufact                 char(50),
    i_size                     char(20),
    i_formulation              char(20),
    i_color                    char(20),
    i_units                    char(10),
    i_container                char(10),
    i_manager_id               integer,
    i_product_name             char(50)
);
--= drop_inventory
DROP TABLE IF EXISTS inventory;
--= inventory
CREATE TABLE inventory (
    inv_date_sk                integer,
    inv_item_sk                integer,
    inv_warehouse_sk           integer,
    inv_quantity_on_hand       integer
);
--= drop_store_sales
DROP TABLE IF EXISTS store_sales;
--= store_sales
CREATE TABLE store_sales (
    ss_sold_date_sk            integer,
    ss_sold_time_sk            integer,
    ss_item_sk                 integer,
    ss_customer_sk             integer,
    ss_cdemo_sk                integer,
    ss_hdemo_sk                integer,
    ss_addr_sk                 integer,
    ss_store_sk                integer,
    ss_promo_sk                integer,
    ss_ticket_number           integer,
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
    ss_net_profit              decimal(7,2)
);
--= drop_store_returns
DROP TABLE IF EXISTS store_returns;
--= store_returns
CREATE TABLE store_returns (
    sr_returned_date_sk        integer,
    sr_return_time_sk          integer,
    sr_item_sk                 integer,
    sr_customer_sk             integer,
    sr_cdemo_sk                integer,
    sr_hdemo_sk                integer,
    sr_addr_sk                 integer,
    sr_store_sk                integer,
    sr_reason_sk               integer,
    sr_ticket_number           integer,
    sr_return_quantity         integer,
    sr_return_amt              decimal(7,2),
    sr_return_tax              decimal(7,2),
    sr_return_amt_inc_tax      decimal(7,2),
    sr_fee                     decimal(7,2),
    sr_return_ship_cost        decimal(7,2),
    sr_refunded_cash           decimal(7,2),
    sr_reversed_charge         decimal(7,2),
    sr_store_credit            decimal(7,2),
    sr_net_loss                decimal(7,2)
);
--= drop_catalog_sales
DROP TABLE IF EXISTS catalog_sales;
--= catalog_sales
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
    cs_item_sk                 integer,
    cs_promo_sk                integer,
    cs_order_number            integer,
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
    cs_net_profit              decimal(7,2)
);
--= drop_catalog_returns
DROP TABLE IF EXISTS catalog_returns;
--= catalog_returns
CREATE TABLE catalog_returns (
    cr_returned_date_sk        integer,
    cr_returned_time_sk        integer,
    cr_item_sk                 integer,
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
    cr_order_number            integer,
    cr_return_quantity         integer,
    cr_return_amount           decimal(7,2),
    cr_return_tax              decimal(7,2),
    cr_return_amt_inc_tax      decimal(7,2),
    cr_fee                     decimal(7,2),
    cr_return_ship_cost        decimal(7,2),
    cr_refunded_cash           decimal(7,2),
    cr_reversed_charge         decimal(7,2),
    cr_store_credit            decimal(7,2),
    cr_net_loss                decimal(7,2)
);
--= drop_web_sales
DROP TABLE IF EXISTS web_sales;
--= web_sales
CREATE TABLE web_sales (
    ws_sold_date_sk            integer,
    ws_sold_time_sk            integer,
    ws_ship_date_sk            integer,
    ws_item_sk                 integer,
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
    ws_order_number            integer,
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
    ws_net_profit              decimal(7,2)
);
--= drop_web_returns
DROP TABLE IF EXISTS web_returns;
--= web_returns
CREATE TABLE web_returns (
    wr_returned_date_sk        integer,
    wr_returned_time_sk        integer,
    wr_item_sk                 integer,
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
    wr_order_number            integer,
    wr_return_quantity         integer,
    wr_return_amt              decimal(7,2),
    wr_return_tax              decimal(7,2),
    wr_return_amt_inc_tax      decimal(7,2),
    wr_fee                     decimal(7,2),
    wr_return_ship_cost        decimal(7,2),
    wr_refunded_cash           decimal(7,2),
    wr_reversed_charge         decimal(7,2),
    wr_account_credit          decimal(7,2),
    wr_net_loss                decimal(7,2)
);
