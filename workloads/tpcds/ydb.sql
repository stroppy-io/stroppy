-- TPC-DS query workload for YDB (YQL), using TPC-DS 4.0.0 Appendix B substitutions.

--= query_1
PRAGMA AnsiImplicitCrossJoin;
$customer_total_return = (select store_returns.sr_customer_sk AS ctr_customer_sk,
  store_returns.sr_store_sk AS ctr_store_sk,
  sum(store_returns.sr_return_amt) AS ctr_total_return
 from store_returns
,date_dim
where sr_returned_date_sk = d_date_sk
and d_year =2000
group by store_returns.sr_customer_sk, store_returns.sr_store_sk
 );
$ctr_store_avg = (select ctr_store_sk AS ctr_store_sk,
  avg(ctr_total_return)*1.2 AS ctr_avg
 from $customer_total_return AS customer_total_return group by ctr_store_sk
 );

 select  c_customer_id
from $customer_total_return AS ctr1
,store
,customer
,$ctr_store_avg AS ctr_store_avg
where ctr1.ctr_total_return > ctr_store_avg.ctr_avg
and ctr_store_avg.ctr_store_sk = ctr1.ctr_store_sk
and s_store_sk = ctr1.ctr_store_sk
and s_state = 'TN'
and ctr1.ctr_customer_sk = c_customer_sk
order by ((c_customer_id) IS NULL), c_customer_id limit 100;

--= query_2
PRAGMA AnsiImplicitCrossJoin;
$wscs = (select sold_date_sk
        ,sales_price
  from (select ws_sold_date_sk sold_date_sk
              ,ws_ext_sales_price sales_price
        from web_sales
        union all
        select cs_sold_date_sk sold_date_sk
              ,cs_ext_sales_price sales_price
        from catalog_sales) AS _sq1);
$wswscs = (select date_dim.d_week_seq AS d_week_seq,
  sum(case when (date_dim.d_day_name='Sunday') then sales_price else null end) AS sun_sales,
  sum(case when (date_dim.d_day_name='Monday') then sales_price else null end) AS mon_sales,
  sum(case when (date_dim.d_day_name='Tuesday') then sales_price else  null end) AS tue_sales,
  sum(case when (date_dim.d_day_name='Wednesday') then sales_price else null end) AS wed_sales,
  sum(case when (date_dim.d_day_name='Thursday') then sales_price else null end) AS thu_sales,
  sum(case when (date_dim.d_day_name='Friday') then sales_price else null end) AS fri_sales,
  sum(case when (date_dim.d_day_name='Saturday') then sales_price else null end) AS sat_sales
 from $wscs AS wscs
     ,date_dim
 where d_date_sk = sold_date_sk
 group by date_dim.d_week_seq
 );

 select d_week_seq1
       ,Math::Round(sun_sales1/sun_sales2, -(2))
       ,Math::Round(mon_sales1/mon_sales2, -(2))
       ,Math::Round(tue_sales1/tue_sales2, -(2))
       ,Math::Round(wed_sales1/wed_sales2, -(2))
       ,Math::Round(thu_sales1/thu_sales2, -(2))
       ,Math::Round(fri_sales1/fri_sales2, -(2))
       ,Math::Round(sat_sales1/sat_sales2, -(2))
 from
 (select wswscs.d_week_seq d_week_seq1
        ,sun_sales sun_sales1
        ,mon_sales mon_sales1
        ,tue_sales tue_sales1
        ,wed_sales wed_sales1
        ,thu_sales thu_sales1
        ,fri_sales fri_sales1
        ,sat_sales sat_sales1
  from $wswscs AS wswscs,date_dim
  where date_dim.d_week_seq = wswscs.d_week_seq and
        d_year = 2001) y,
 (select wswscs.d_week_seq d_week_seq2
        ,sun_sales sun_sales2
        ,mon_sales mon_sales2
        ,tue_sales tue_sales2
        ,wed_sales wed_sales2
        ,thu_sales thu_sales2
        ,fri_sales fri_sales2
        ,sat_sales sat_sales2
  from $wswscs AS wswscs
      ,date_dim
  where date_dim.d_week_seq = wswscs.d_week_seq and
        d_year = 2001+1) z
 where d_week_seq1=d_week_seq2-53
 order by ((d_week_seq1) IS NULL), d_week_seq1;

--= query_3
PRAGMA AnsiImplicitCrossJoin;
select dt.d_year AS d_year,
  item.i_brand_id AS brand_id,
  item.i_brand AS brand,
  sum(store_sales.ss_ext_sales_price) AS sum_agg
 from  date_dim dt
      ,store_sales
      ,item
 where dt.d_date_sk = store_sales.ss_sold_date_sk
   and store_sales.ss_item_sk = item.i_item_sk
   and item.i_manufact_id = 128
   and dt.d_moy=11
 group by dt.d_year, item.i_brand, item.i_brand_id
 order by ((d_year) IS NULL), d_year, ((sum_agg) IS NOT NULL), sum_agg DESC, ((brand_id) IS NULL), brand_id limit 100;

--= query_4
PRAGMA AnsiImplicitCrossJoin;
$year_total = (select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  customer.c_preferred_cust_flag AS customer_preferred_cust_flag,
  customer.c_birth_country AS customer_birth_country,
  customer.c_login AS customer_login,
  customer.c_email_address AS customer_email_address,
  date_dim.d_year AS dyear,
  sum(((store_sales.ss_ext_list_price-store_sales.ss_ext_wholesale_cost-store_sales.ss_ext_discount_amt)+store_sales.ss_ext_sales_price)/2) AS year_total,
  's' AS sale_type
 from customer
     ,store_sales
     ,date_dim
 where c_customer_sk = ss_customer_sk
   and ss_sold_date_sk = d_date_sk
 group by customer.c_customer_id, customer.c_first_name, customer.c_last_name, customer.c_preferred_cust_flag, customer.c_birth_country, customer.c_login, customer.c_email_address, date_dim.d_year
 union all
 select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  customer.c_preferred_cust_flag AS customer_preferred_cust_flag,
  customer.c_birth_country AS customer_birth_country,
  customer.c_login AS customer_login,
  customer.c_email_address AS customer_email_address,
  date_dim.d_year AS dyear,
  sum((((catalog_sales.cs_ext_list_price-catalog_sales.cs_ext_wholesale_cost-catalog_sales.cs_ext_discount_amt)+catalog_sales.cs_ext_sales_price)/2) ) AS year_total,
  'c' AS sale_type
 from customer
     ,catalog_sales
     ,date_dim
 where c_customer_sk = cs_bill_customer_sk
   and cs_sold_date_sk = d_date_sk
 group by customer.c_customer_id, customer.c_first_name, customer.c_last_name, customer.c_preferred_cust_flag, customer.c_birth_country, customer.c_login, customer.c_email_address, date_dim.d_year
 union all
 select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  customer.c_preferred_cust_flag AS customer_preferred_cust_flag,
  customer.c_birth_country AS customer_birth_country,
  customer.c_login AS customer_login,
  customer.c_email_address AS customer_email_address,
  date_dim.d_year AS dyear,
  sum((((web_sales.ws_ext_list_price-web_sales.ws_ext_wholesale_cost-web_sales.ws_ext_discount_amt)+web_sales.ws_ext_sales_price)/2) ) AS year_total,
  'w' AS sale_type
 from customer
     ,web_sales
     ,date_dim
 where c_customer_sk = ws_bill_customer_sk
   and ws_sold_date_sk = d_date_sk
 group by customer.c_customer_id, customer.c_first_name, customer.c_last_name, customer.c_preferred_cust_flag, customer.c_birth_country, customer.c_login, customer.c_email_address, date_dim.d_year
 );

  select
                  t_s_secyear.customer_id AS customer_id
                 ,t_s_secyear.customer_first_name AS customer_first_name
                 ,t_s_secyear.customer_last_name AS customer_last_name
                 ,t_s_secyear.customer_preferred_cust_flag AS customer_preferred_cust_flag
 from $year_total AS t_s_firstyear
     ,$year_total AS t_s_secyear
     ,$year_total AS t_c_firstyear
     ,$year_total AS t_c_secyear
     ,$year_total AS t_w_firstyear
     ,$year_total AS t_w_secyear
 where t_s_secyear.customer_id = t_s_firstyear.customer_id
   and t_s_firstyear.customer_id = t_c_secyear.customer_id
   and t_s_firstyear.customer_id = t_c_firstyear.customer_id
   and t_s_firstyear.customer_id = t_w_firstyear.customer_id
   and t_s_firstyear.customer_id = t_w_secyear.customer_id
   and t_s_firstyear.sale_type = 's'
   and t_c_firstyear.sale_type = 'c'
   and t_w_firstyear.sale_type = 'w'
   and t_s_secyear.sale_type = 's'
   and t_c_secyear.sale_type = 'c'
   and t_w_secyear.sale_type = 'w'
   and t_s_firstyear.dyear =  2001
   and t_s_secyear.dyear = 2001+1
   and t_c_firstyear.dyear =  2001
   and t_c_secyear.dyear =  2001+1
   and t_w_firstyear.dyear = 2001
   and t_w_secyear.dyear = 2001+1
   and t_s_firstyear.year_total > 0
   and t_c_firstyear.year_total > 0
   and t_w_firstyear.year_total > 0
   and case when t_c_firstyear.year_total > 0 then t_c_secyear.year_total / t_c_firstyear.year_total else null end
           > case when t_s_firstyear.year_total > 0 then t_s_secyear.year_total / t_s_firstyear.year_total else null end
   and case when t_c_firstyear.year_total > 0 then t_c_secyear.year_total / t_c_firstyear.year_total else null end
           > case when t_w_firstyear.year_total > 0 then t_w_secyear.year_total / t_w_firstyear.year_total else null end
 order by ((t_s_secyear.customer_id) IS NULL), t_s_secyear.customer_id, ((t_s_secyear.customer_first_name) IS NULL), t_s_secyear.customer_first_name, ((t_s_secyear.customer_last_name) IS NULL), t_s_secyear.customer_last_name, ((t_s_secyear.customer_preferred_cust_flag) IS NULL), t_s_secyear.customer_preferred_cust_flag limit 100;

--= query_5
PRAGMA AnsiImplicitCrossJoin;
$ssr = (select store.s_store_id AS s_store_id,
  sum(sales_price) AS sales,
  sum(profit) AS profit,
  sum(return_amt) AS returns,
  sum(net_loss) AS profit_loss
 from
  ( select  ss_store_sk as store_sk,
            ss_sold_date_sk  as date_sk,
            ss_ext_sales_price as sales_price,
            ss_net_profit as profit,
            cast(0 as Double) as return_amt,
            cast(0 as Double) as net_loss
    from store_sales
    union all
    select sr_store_sk as store_sk,
           sr_returned_date_sk as date_sk,
           cast(0 as Double) as sales_price,
           cast(0 as Double) as profit,
           sr_return_amt as return_amt,
           sr_net_loss as net_loss
    from store_returns
   ) salesreturns,
     date_dim,
     store
 where date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-06'
       and store_sk = s_store_sk
 group by store.s_store_id
 );
$csr = (select catalog_page.cp_catalog_page_id AS cp_catalog_page_id,
  sum(sales_price) AS sales,
  sum(profit) AS profit,
  sum(return_amt) AS returns,
  sum(net_loss) AS profit_loss
 from
  ( select  cs_catalog_page_sk as page_sk,
            cs_sold_date_sk  as date_sk,
            cs_ext_sales_price as sales_price,
            cs_net_profit as profit,
            cast(0 as Double) as return_amt,
            cast(0 as Double) as net_loss
    from catalog_sales
    union all
    select cr_catalog_page_sk as page_sk,
           cr_returned_date_sk as date_sk,
           cast(0 as Double) as sales_price,
           cast(0 as Double) as profit,
           cr_return_amount as return_amt,
           cr_net_loss as net_loss
    from catalog_returns
   ) salesreturns,
     date_dim,
     catalog_page
 where date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-06'
       and page_sk = cp_catalog_page_sk
 group by catalog_page.cp_catalog_page_id
 );
$wsr = (select web_site.web_site_id AS web_site_id,
  sum(sales_price) AS sales,
  sum(profit) AS profit,
  sum(return_amt) AS returns,
  sum(net_loss) AS profit_loss
 from
  ( select  ws_web_site_sk as wsr_web_site_sk,
            ws_sold_date_sk  as date_sk,
            ws_ext_sales_price as sales_price,
            ws_net_profit as profit,
            cast(0 as Double) as return_amt,
            cast(0 as Double) as net_loss
    from web_sales
    union all
    select ws_web_site_sk as wsr_web_site_sk,
           wr_returned_date_sk as date_sk,
           cast(0 as Double) as sales_price,
           cast(0 as Double) as profit,
           wr_return_amt as return_amt,
           wr_net_loss as net_loss
    from web_returns left outer join web_sales on
         ( web_returns.wr_item_sk = web_sales.ws_item_sk
           and web_returns.wr_order_number = web_sales.ws_order_number)
   ) salesreturns,
     date_dim,
     web_site
 where date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-06'
       and wsr_web_site_sk = web_site_sk
 group by web_site.web_site_id
 );

  select channel AS channel,
  id AS id,
  sum(sales) AS sales,
  sum(returns) AS returns,
  sum(profit) AS profit
 from
 (select 'store channel' as channel
        , 'store' || s_store_id as id
        , sales
        , returns
        , (profit - profit_loss) as profit
 from   $ssr AS ssr
 union all
 select 'catalog channel' as channel
        , 'catalog_page' || cp_catalog_page_id as id
        , sales
        , returns
        , (profit - profit_loss) as profit
 from  $csr AS csr
 union all
 select 'web channel' as channel
        , 'web_site' || web_site_id as id
        , sales
        , returns
        , (profit - profit_loss) as profit
 from   $wsr AS wsr
 ) x
 group by  rollup (channel, id)

 order by ((channel) IS NULL), channel, ((id) IS NULL), id limit 100;

--= query_6
PRAGMA AnsiImplicitCrossJoin;
select a.ca_state AS state,
  count(*) AS cnt
 from customer_address a
     ,customer c
     ,store_sales s
     ,date_dim d
     ,item i
     ,(select i_category AS iavg_cat,
  avg(i_current_price) AS iavg_price
 from item group by i_category
 ) iavg
 where       a.ca_address_sk = c.c_current_addr_sk
    and c.c_customer_sk = s.ss_customer_sk
    and s.ss_sold_date_sk = d.d_date_sk
    and s.ss_item_sk = i.i_item_sk
    and d.d_month_seq =
         (select distinct (d_month_seq)
          from date_dim
               where d_year = 2001
            and d_moy = 1 )
    and iavg.iavg_cat = i.i_category
    and i.i_current_price > 1.2 * iavg.iavg_price
 group by a.ca_state
 having count(*) >= 10
 order by ((cnt) IS NULL), cnt, ((state) IS NULL), state limit 100;

--= query_7
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  avg(store_sales.ss_quantity) AS agg1,
  avg(store_sales.ss_list_price) AS agg2,
  avg(store_sales.ss_coupon_amt) AS agg3,
  avg(store_sales.ss_sales_price) AS agg4
 from store_sales, customer_demographics, date_dim, item, promotion
 where ss_sold_date_sk = d_date_sk and
       ss_item_sk = i_item_sk and
       ss_cdemo_sk = cd_demo_sk and
       ss_promo_sk = p_promo_sk and
       cd_gender = 'M' and
       cd_marital_status = 'S' and
       cd_education_status = 'College' and
       (p_channel_email = 'N' or p_channel_event = 'N') and
       d_year = 2000
 group by item.i_item_id
 order by ((i_item_id) IS NULL), i_item_id limit 100;

--= query_8
PRAGMA AnsiImplicitCrossJoin;
select store.s_store_name AS s_store_name,
  sum(store_sales.ss_net_profit) AS c1
 from store_sales
     ,date_dim
     ,store,
     (select ca_zip
     from (
      SELECT Unicode::Substring(ca_zip, CAST(0 AS Uint32), CAST(5 AS Uint32)) ca_zip
      FROM customer_address
      WHERE Unicode::Substring(ca_zip, CAST(0 AS Uint32), CAST(5 AS Uint32)) IN (
                          '24128','76232','35863','28577','55565','17183',
                          '54601','67897','22752','86284','18376',
                          '38607','45200','21756','29741','96765',
                          '23932','89360','29839','25989','28898',
                          '91068','72550','10390','18845','47770',
                          '82636','41367','76638','86198','81312',
                          '37126','39192','88424','72175','81426',
                          '53672','10445','42666','66864','66708',
                          '41248','48583','82276','18842','78890',
                          '49448','14089','38122','34425','79077',
                          '19849','43285','39861','66162','77610',
                          '13695','99543','83444','83041','12305',
                          '57665','68341','25003','57834','62878',
                          '49130','81096','18840','27700','23470',
                          '50412','21195','16021','76107','71954',
                          '68309','18119','98359','64544','10336',
                          '86379','27068','39736','98569','28915',
                          '24206','56529','57647','54917','42961',
                          '91110','63981','14922','36420','23006',
                          '67467','32754','30903','20260','31671',
                          '51798','72325','85816','68621','13955',
                          '36446','41766','68806','16725','15146',
                          '22744','35850','88086','51649','18270',
                          '52867','39972','96976','63792','11376',
                          '94898','13595','10516','90225','58943',
                          '39371','94945','28587','96576','57855',
                          '28488','26105','83933','25858','34322',
                          '44438','73171','30122','34102','22685',
                          '71256','78451','54364','13354','45375',
                          '40558','56458','28286','45266','47305',
                          '69399','83921','26233','11101','15371',
                          '69913','35942','15882','25631','24610',
                          '44165','99076','33786','70738','26653',
                          '14328','72305','62496','22152','10144',
                          '64147','48425','14663','21076','18799',
                          '30450','63089','81019','68893','24996',
                          '51200','51211','45692','92712','70466',
                          '79994','22437','25280','38935','71791',
                          '73134','56571','14060','19505','72425',
                          '56575','74351','68786','51650','20004',
                          '18383','76614','11634','18906','15765',
                          '41368','73241','76698','78567','97189',
                          '28545','76231','75691','22246','51061',
                          '90578','56691','68014','51103','94167',
                          '57047','14867','73520','15734','63435',
                          '25733','35474','24676','94627','53535',
                          '17879','15559','53268','59166','11928',
                          '59402','33282','45721','43933','68101',
                          '33515','36634','71286','19736','58058',
                          '55253','67473','41918','19515','36495',
                          '19430','22351','77191','91393','49156',
                          '50298','87501','18652','53179','18767',
                          '63193','23968','65164','68880','21286',
                          '72823','58470','67301','13394','31016',
                          '70372','67030','40604','24317','45748',
                          '39127','26065','77721','31029','31880',
                          '60576','24671','45549','13376','50016',
                          '33123','19769','22927','97789','46081',
                          '72151','15723','46136','51949','68100',
                          '96888','64528','14171','79777','28709',
                          '11489','25103','32213','78668','22245',
                          '15798','27156','37930','62971','21337',
                          '51622','67853','10567','38415','15455',
                          '58263','42029','60279','37125','56240',
                          '88190','50308','26859','64457','89091',
                          '82136','62377','36233','63837','58078',
                          '17043','30010','60099','28810','98025',
                          '29178','87343','73273','30469','64034',
                          '39516','86057','21309','90257','67875',
                          '40162','11356','73650','61810','72013',
                          '30431','22461','19512','13375','55307',
                          '30625','83849','68908','26689','96451',
                          '38193','46820','88885','84935','69035',
                          '83144','47537','56616','94983','48033',
                          '69952','25486','61547','27385','61860',
                          '58048','56910','16807','17871','35258',
                          '31387','35458','22815','56600','68730',
                          '48017','95694','32919','88217','27116',
                          '28239','58032','18884','16791','21343',
                          '97462','18569','75660','15475')
     intersect
      select ca_zip
      from (SELECT Unicode::Substring(customer_address.ca_zip, CAST(0 AS Uint32), CAST(5 AS Uint32)) AS ca_zip,
  count(*) AS cnt
 FROM customer_address, customer
            WHERE ca_address_sk = c_current_addr_sk and
                  c_preferred_cust_flag='Y'
            group by customer_address.ca_zip
 having count(*) > 10)A1)A2) V1
 where ss_store_sk = s_store_sk
  and ss_sold_date_sk = d_date_sk
  and d_qoy = 2 and d_year = 1998
  and (Unicode::Substring(s_zip, CAST(0 AS Uint32), CAST(2 AS Uint32)) = Unicode::Substring(V1.ca_zip, CAST(0 AS Uint32), CAST(2 AS Uint32)))
 group by store.s_store_name
 order by ((s_store_name) IS NULL), s_store_name limit 100;

--= query_9
PRAGMA AnsiImplicitCrossJoin;
select case when (select count(*)
                  from store_sales
                  where ss_quantity between 1 and 20) > 74129
            then (select avg(ss_ext_discount_amt)
                  from store_sales
                  where ss_quantity between 1 and 20)
            else (select avg(ss_net_paid)
                  from store_sales
                  where ss_quantity between 1 and 20) end bucket1 ,
       case when (select count(*)
                  from store_sales
                  where ss_quantity between 21 and 40) > 122840
            then (select avg(ss_ext_discount_amt)
                  from store_sales
                  where ss_quantity between 21 and 40)
            else (select avg(ss_net_paid)
                  from store_sales
                  where ss_quantity between 21 and 40) end bucket2,
       case when (select count(*)
                  from store_sales
                  where ss_quantity between 41 and 60) > 56580
            then (select avg(ss_ext_discount_amt)
                  from store_sales
                  where ss_quantity between 41 and 60)
            else (select avg(ss_net_paid)
                  from store_sales
                  where ss_quantity between 41 and 60) end bucket3,
       case when (select count(*)
                  from store_sales
                  where ss_quantity between 61 and 80) > 10097
            then (select avg(ss_ext_discount_amt)
                  from store_sales
                  where ss_quantity between 61 and 80)
            else (select avg(ss_net_paid)
                  from store_sales
                  where ss_quantity between 61 and 80) end bucket4,
       case when (select count(*)
                  from store_sales
                  where ss_quantity between 81 and 100) > 165306
            then (select avg(ss_ext_discount_amt)
                  from store_sales
                  where ss_quantity between 81 and 100)
            else (select avg(ss_net_paid)
                  from store_sales
                  where ss_quantity between 81 and 100) end bucket5
from reason
where r_reason_sk = 1;

--= query_10
PRAGMA AnsiImplicitCrossJoin;
select customer_demographics.cd_gender AS cd_gender,
  customer_demographics.cd_marital_status AS cd_marital_status,
  customer_demographics.cd_education_status AS cd_education_status,
  count(*) AS cnt1,
  customer_demographics.cd_purchase_estimate AS cd_purchase_estimate,
  count(*) AS cnt2,
  customer_demographics.cd_credit_rating AS cd_credit_rating,
  count(*) AS cnt3,
  customer_demographics.cd_dep_count AS cd_dep_count,
  count(*) AS cnt4,
  customer_demographics.cd_dep_employed_count AS cd_dep_employed_count,
  count(*) AS cnt5,
  customer_demographics.cd_dep_college_count AS cd_dep_college_count,
  count(*) AS cnt6
 from
  customer c,customer_address ca,customer_demographics
 where
  c.c_current_addr_sk = ca.ca_address_sk and
  ca_county in ('Rush County','Toole County','Jefferson County','Dona Ana County','La Porte County') and
  cd_demo_sk = c.c_current_cdemo_sk and
  c.c_customer_sk in (select ss_customer_sk from store_sales,date_dim where ss_sold_date_sk = d_date_sk and
                d_year = 2002 and
                d_moy between 1 and 1+3) and
   (c.c_customer_sk in (select ws_bill_customer_sk from web_sales,date_dim where ws_sold_date_sk = d_date_sk and
                  d_year = 2002 and
                  d_moy between 1 ANd 1+3) or
    c.c_customer_sk in (select cs_ship_customer_sk from catalog_sales,date_dim where cs_sold_date_sk = d_date_sk and
                  d_year = 2002 and
                  d_moy between 1 and 1+3))
 group by customer_demographics.cd_gender, customer_demographics.cd_marital_status, customer_demographics.cd_education_status, customer_demographics.cd_purchase_estimate, customer_demographics.cd_credit_rating, customer_demographics.cd_dep_count, customer_demographics.cd_dep_employed_count, customer_demographics.cd_dep_college_count
 order by ((cd_gender) IS NULL), cd_gender, ((cd_marital_status) IS NULL), cd_marital_status, ((cd_education_status) IS NULL), cd_education_status, ((cd_purchase_estimate) IS NULL), cd_purchase_estimate, ((cd_credit_rating) IS NULL), cd_credit_rating, ((cd_dep_count) IS NULL), cd_dep_count, ((cd_dep_employed_count) IS NULL), cd_dep_employed_count, ((cd_dep_college_count) IS NULL), cd_dep_college_count limit 100;

--= query_11
PRAGMA AnsiImplicitCrossJoin;
$year_total = (select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  customer.c_preferred_cust_flag AS customer_preferred_cust_flag,
  customer.c_birth_country AS customer_birth_country,
  customer.c_login AS customer_login,
  customer.c_email_address AS customer_email_address,
  date_dim.d_year AS dyear,
  sum(store_sales.ss_ext_list_price-store_sales.ss_ext_discount_amt) AS year_total,
  's' AS sale_type
 from customer
     ,store_sales
     ,date_dim
 where c_customer_sk = ss_customer_sk
   and ss_sold_date_sk = d_date_sk
 group by customer.c_customer_id, customer.c_first_name, customer.c_last_name, customer.c_preferred_cust_flag, customer.c_birth_country, customer.c_login, customer.c_email_address, date_dim.d_year
 union all
 select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  customer.c_preferred_cust_flag AS customer_preferred_cust_flag,
  customer.c_birth_country AS customer_birth_country,
  customer.c_login AS customer_login,
  customer.c_email_address AS customer_email_address,
  date_dim.d_year AS dyear,
  sum(web_sales.ws_ext_list_price-web_sales.ws_ext_discount_amt) AS year_total,
  'w' AS sale_type
 from customer
     ,web_sales
     ,date_dim
 where c_customer_sk = ws_bill_customer_sk
   and ws_sold_date_sk = d_date_sk
 group by customer.c_customer_id, customer.c_first_name, customer.c_last_name, customer.c_preferred_cust_flag, customer.c_birth_country, customer.c_login, customer.c_email_address, date_dim.d_year
 );

  select
                  t_s_secyear.customer_id AS customer_id
                 ,t_s_secyear.customer_first_name AS customer_first_name
                 ,t_s_secyear.customer_last_name AS customer_last_name
                 ,t_s_secyear.customer_preferred_cust_flag AS customer_preferred_cust_flag
 from $year_total AS t_s_firstyear
     ,$year_total AS t_s_secyear
     ,$year_total AS t_w_firstyear
     ,$year_total AS t_w_secyear
 where t_s_secyear.customer_id = t_s_firstyear.customer_id
         and t_s_firstyear.customer_id = t_w_secyear.customer_id
         and t_s_firstyear.customer_id = t_w_firstyear.customer_id
         and t_s_firstyear.sale_type = 's'
         and t_w_firstyear.sale_type = 'w'
         and t_s_secyear.sale_type = 's'
         and t_w_secyear.sale_type = 'w'
         and t_s_firstyear.dyear = 2001
         and t_s_secyear.dyear = 2001+1
         and t_w_firstyear.dyear = 2001
         and t_w_secyear.dyear = 2001+1
         and t_s_firstyear.year_total > 0
         and t_w_firstyear.year_total > 0
         and case when t_w_firstyear.year_total > 0 then t_w_secyear.year_total / t_w_firstyear.year_total else 0.0 end
             > case when t_s_firstyear.year_total > 0 then t_s_secyear.year_total / t_s_firstyear.year_total else 0.0 end
 order by ((t_s_secyear.customer_id) IS NULL), t_s_secyear.customer_id, ((t_s_secyear.customer_first_name) IS NULL), t_s_secyear.customer_first_name, ((t_s_secyear.customer_last_name) IS NULL), t_s_secyear.customer_last_name, ((t_s_secyear.customer_preferred_cust_flag) IS NULL), t_s_secyear.customer_preferred_cust_flag limit 100;

--= query_12
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  item.i_category AS i_category,
  item.i_class AS i_class,
  item.i_current_price AS i_current_price,
  sum(web_sales.ws_ext_sales_price) AS itemrevenue,
  sum(web_sales.ws_ext_sales_price)*100/sum(sum(web_sales.ws_ext_sales_price)) over
          (partition by item.i_class) AS revenueratio
 from
    web_sales
        ,item
        ,date_dim
where
    ws_item_sk = i_item_sk
    and i_category in ('Sports', 'Books', 'Home')
    and ws_sold_date_sk = d_date_sk
    and d_date between ('1999-02-22')
                and '1999-03-24'
group by item.i_item_id, item.i_item_desc, item.i_category, item.i_class, item.i_current_price
 order by ((i_category) IS NULL), i_category, ((i_class) IS NULL), i_class, ((i_item_id) IS NULL), i_item_id, ((i_item_desc) IS NULL), i_item_desc, ((revenueratio) IS NULL), revenueratio limit 100;

--= query_13
PRAGMA AnsiImplicitCrossJoin;
select avg(store_sales.ss_quantity) AS c2,
  avg(store_sales.ss_ext_sales_price) AS c3,
  avg(store_sales.ss_ext_wholesale_cost) AS c4,
  sum(store_sales.ss_ext_wholesale_cost) AS c5
 from store_sales
     ,store
     ,customer_demographics
     ,household_demographics
     ,customer_address
     ,date_dim
 where s_store_sk = ss_store_sk
 and  ss_sold_date_sk = d_date_sk and d_year = 2001
 and((ss_hdemo_sk=hd_demo_sk
  and cd_demo_sk = ss_cdemo_sk
  and cd_marital_status = 'M'
  and cd_education_status = 'Advanced Degree'
  and ss_sales_price between 100.00 and 150.00
  and hd_dep_count = 3
     )or
     (ss_hdemo_sk=hd_demo_sk
  and cd_demo_sk = ss_cdemo_sk
  and cd_marital_status = 'S'
  and cd_education_status = 'College'
  and ss_sales_price between 50.00 and 100.00
  and hd_dep_count = 1
     ) or
     (ss_hdemo_sk=hd_demo_sk
  and cd_demo_sk = ss_cdemo_sk
  and cd_marital_status = 'W'
  and cd_education_status = '2 yr Degree'
  and ss_sales_price between 150.00 and 200.00
  and hd_dep_count = 1
     ))
 and((ss_addr_sk = ca_address_sk
  and ca_country = 'United States'
  and ca_state in ('TX', 'OH', 'TX')
  and ss_net_profit between 100 and 200
     ) or
     (ss_addr_sk = ca_address_sk
  and ca_country = 'United States'
  and ca_state in ('OR', 'NM', 'KY')
  and ss_net_profit between 150 and 300
     ) or
     (ss_addr_sk = ca_address_sk
  and ca_country = 'United States'
  and ca_state in ('MS', 'SC', 'IA')
  and ss_net_profit between 50 and 250
     ));

--= query_14_a
PRAGMA AnsiImplicitCrossJoin;
$cross_items = (select i_item_sk ss_item_sk
 from item,
 (select iss.i_brand_id brand_id
     ,iss.i_class_id class_id
     ,iss.i_category_id category_id
 from store_sales
     ,item iss
     ,date_dim d1
 where ss_item_sk = iss.i_item_sk
   and ss_sold_date_sk = d1.d_date_sk
   and d1.d_year between 1999 AND 1999 + 2
 intersect
 select ics.i_brand_id AS brand_id
     ,ics.i_class_id AS class_id
     ,ics.i_category_id AS category_id
 from catalog_sales
     ,item ics
     ,date_dim d2
 where cs_item_sk = ics.i_item_sk
   and cs_sold_date_sk = d2.d_date_sk
   and d2.d_year between 1999 AND 1999 + 2
 intersect
 select iws.i_brand_id AS brand_id
     ,iws.i_class_id AS class_id
     ,iws.i_category_id AS category_id
 from web_sales
     ,item iws
     ,date_dim d3
 where ws_item_sk = iws.i_item_sk
   and ws_sold_date_sk = d3.d_date_sk
   and d3.d_year between 1999 AND 1999 + 2) AS _sq1
 where i_brand_id = brand_id
      and i_class_id = class_id
      and i_category_id = category_id);
$avg_sales = (select avg(quantity*list_price) average_sales
  from (select ss_quantity quantity
             ,ss_list_price list_price
       from store_sales
           ,date_dim
       where ss_sold_date_sk = d_date_sk
         and d_year between 1999 and 1999 + 2
       union all
       select cs_quantity quantity
             ,cs_list_price list_price
       from catalog_sales
           ,date_dim
       where cs_sold_date_sk = d_date_sk
         and d_year between 1999 and 1999 + 2
       union all
       select ws_quantity quantity
             ,ws_list_price list_price
       from web_sales
           ,date_dim
       where ws_sold_date_sk = d_date_sk
         and d_year between 1999 and 1999 + 2) x);

  select channel AS channel,
  i_brand_id AS i_brand_id,
  i_class_id AS i_class_id,
  i_category_id AS i_category_id,
  sum(sales) AS c6,
  sum(number_sales) AS c7
 from(
       select 'store' AS channel,
  item.i_brand_id AS i_brand_id,
  item.i_class_id AS i_class_id,
  item.i_category_id AS i_category_id,
  sum(store_sales.ss_quantity*store_sales.ss_list_price) AS sales,
  count(*) AS number_sales
 from store_sales
           ,item
           ,date_dim
       where ss_item_sk in (select ss_item_sk from $cross_items AS cross_items)
         and ss_item_sk = i_item_sk
         and ss_sold_date_sk = d_date_sk
         and d_year = 1999+2
         and d_moy = 11
       group by item.i_brand_id, item.i_class_id, item.i_category_id
 having sum(store_sales.ss_quantity*store_sales.ss_list_price) > (select average_sales from $avg_sales AS avg_sales)
       union all
       select 'catalog' AS channel,
  item.i_brand_id AS i_brand_id,
  item.i_class_id AS i_class_id,
  item.i_category_id AS i_category_id,
  sum(catalog_sales.cs_quantity*catalog_sales.cs_list_price) AS sales,
  count(*) AS number_sales
 from catalog_sales
           ,item
           ,date_dim
       where cs_item_sk in (select ss_item_sk from $cross_items AS cross_items)
         and cs_item_sk = i_item_sk
         and cs_sold_date_sk = d_date_sk
         and d_year = 1999+2
         and d_moy = 11
       group by item.i_brand_id, item.i_class_id, item.i_category_id
 having sum(catalog_sales.cs_quantity*catalog_sales.cs_list_price) > (select average_sales from $avg_sales AS avg_sales)
       union all
       select 'web' AS channel,
  item.i_brand_id AS i_brand_id,
  item.i_class_id AS i_class_id,
  item.i_category_id AS i_category_id,
  sum(web_sales.ws_quantity*web_sales.ws_list_price) AS sales,
  count(*) AS number_sales
 from web_sales
           ,item
           ,date_dim
       where ws_item_sk in (select ss_item_sk from $cross_items AS cross_items)
         and ws_item_sk = i_item_sk
         and ws_sold_date_sk = d_date_sk
         and d_year = 1999+2
         and d_moy = 11
       group by item.i_brand_id, item.i_class_id, item.i_category_id
 having sum(web_sales.ws_quantity*web_sales.ws_list_price) > (select average_sales from $avg_sales AS avg_sales)
 ) y
 group by  rollup (channel, i_brand_id,i_class_id,i_category_id)

 order by ((channel) IS NULL), channel, ((i_brand_id) IS NULL), i_brand_id, ((i_class_id) IS NULL), i_class_id, ((i_category_id) IS NULL), i_category_id limit 100;

--= query_14_b
PRAGMA AnsiImplicitCrossJoin;
$cross_items = (select i_item_sk ss_item_sk
 from item,
 (select iss.i_brand_id brand_id
     ,iss.i_class_id class_id
     ,iss.i_category_id category_id
 from store_sales
     ,item iss
     ,date_dim d1
 where ss_item_sk = iss.i_item_sk
   and ss_sold_date_sk = d1.d_date_sk
   and d1.d_year between 1999 AND 1999 + 2
 intersect
 select ics.i_brand_id AS brand_id
     ,ics.i_class_id AS class_id
     ,ics.i_category_id AS category_id
 from catalog_sales
     ,item ics
     ,date_dim d2
 where cs_item_sk = ics.i_item_sk
   and cs_sold_date_sk = d2.d_date_sk
   and d2.d_year between 1999 AND 1999 + 2
 intersect
 select iws.i_brand_id AS brand_id
     ,iws.i_class_id AS class_id
     ,iws.i_category_id AS category_id
 from web_sales
     ,item iws
     ,date_dim d3
 where ws_item_sk = iws.i_item_sk
   and ws_sold_date_sk = d3.d_date_sk
   and d3.d_year between 1999 AND 1999 + 2) x
 where i_brand_id = brand_id
      and i_class_id = class_id
      and i_category_id = category_id);
$avg_sales = (select avg(quantity*list_price) average_sales
  from (select ss_quantity quantity
             ,ss_list_price list_price
       from store_sales
           ,date_dim
       where ss_sold_date_sk = d_date_sk
         and d_year between 1999 and 1999 + 2
       union all
       select cs_quantity quantity
             ,cs_list_price list_price
       from catalog_sales
           ,date_dim
       where cs_sold_date_sk = d_date_sk
         and d_year between 1999 and 1999 + 2
       union all
       select ws_quantity quantity
             ,ws_list_price list_price
       from web_sales
           ,date_dim
       where ws_sold_date_sk = d_date_sk
         and d_year between 1999 and 1999 + 2) x);

  select  this_year.channel ty_channel
                           ,this_year.i_brand_id ty_brand
                           ,this_year.i_class_id ty_class
                           ,this_year.i_category_id ty_category
                           ,this_year.sales ty_sales
                           ,this_year.number_sales ty_number_sales
                           ,last_year.channel ly_channel
                           ,last_year.i_brand_id ly_brand
                           ,last_year.i_class_id ly_class
                           ,last_year.i_category_id ly_category
                           ,last_year.sales ly_sales
                           ,last_year.number_sales ly_number_sales
 from
 (select 'store' AS channel,
  item.i_brand_id AS i_brand_id,
  item.i_class_id AS i_class_id,
  item.i_category_id AS i_category_id,
  sum(store_sales.ss_quantity*store_sales.ss_list_price) AS sales,
  count(*) AS number_sales
 from store_sales
     ,item
     ,date_dim
 where ss_item_sk in (select ss_item_sk from $cross_items AS cross_items)
   and ss_item_sk = i_item_sk
   and ss_sold_date_sk = d_date_sk
   and d_week_seq = (select d_week_seq
                     from date_dim
                     where d_year = 1999 + 1
                       and d_moy = 12
                       and d_dom = 11)
 group by item.i_brand_id, item.i_class_id, item.i_category_id
 having sum(store_sales.ss_quantity*store_sales.ss_list_price) > (select average_sales from $avg_sales AS avg_sales)) this_year,
 (select 'store' AS channel,
  item.i_brand_id AS i_brand_id,
  item.i_class_id AS i_class_id,
  item.i_category_id AS i_category_id,
  sum(store_sales.ss_quantity*store_sales.ss_list_price) AS sales,
  count(*) AS number_sales
 from store_sales
     ,item
     ,date_dim
 where ss_item_sk in (select ss_item_sk from $cross_items AS cross_items)
   and ss_item_sk = i_item_sk
   and ss_sold_date_sk = d_date_sk
   and d_week_seq = (select d_week_seq
                     from date_dim
                     where d_year = 1999
                       and d_moy = 12
                       and d_dom = 11)
 group by item.i_brand_id, item.i_class_id, item.i_category_id
 having sum(store_sales.ss_quantity*store_sales.ss_list_price) > (select average_sales from $avg_sales AS avg_sales)) last_year
 where this_year.i_brand_id= last_year.i_brand_id
   and this_year.i_class_id = last_year.i_class_id
   and this_year.i_category_id = last_year.i_category_id
 order by ((this_year.channel) IS NULL), this_year.channel, ((this_year.i_brand_id) IS NULL), this_year.i_brand_id, ((this_year.i_class_id) IS NULL), this_year.i_class_id, ((this_year.i_category_id) IS NULL), this_year.i_category_id limit 100;

--= query_15
PRAGMA AnsiImplicitCrossJoin;
select customer_address.ca_zip AS ca_zip,
  sum(catalog_sales.cs_sales_price) AS c8
 from catalog_sales
     ,customer
     ,customer_address
     ,date_dim
 where cs_bill_customer_sk = c_customer_sk
    and c_current_addr_sk = ca_address_sk
    and ( Unicode::Substring(ca_zip, CAST(0 AS Uint32), CAST(5 AS Uint32)) in ('85669', '86197','88274','83405','86475',
                                   '85392', '85460', '80348', '81792')
          or ca_state in ('CA','WA','GA')
          or cs_sales_price > 500)
    and cs_sold_date_sk = d_date_sk
    and d_qoy = 2 and d_year = 2001
 group by customer_address.ca_zip
 order by ((ca_zip) IS NULL), ca_zip limit 100;

--= query_16
PRAGMA AnsiImplicitCrossJoin;
select count(distinct cs1.cs_order_number) AS `order count`,
  sum(cs1.cs_ext_ship_cost) AS `total shipping cost`,
  sum(cs1.cs_net_profit) AS `total net profit`
 from catalog_sales cs1, date_dim, customer_address, call_center
where d_date between '2002-02-01' and '2002-04-02'
  and cs1.cs_ship_date_sk = d_date_sk
  and cs1.cs_ship_addr_sk = ca_address_sk
  and ca_state = 'GA'
  and cs1.cs_call_center_sk = cc_call_center_sk
  and cc_county in ('Williamson County','Williamson County','Williamson County','Williamson County','Williamson County')
  and cs1.cs_order_number in (select cs_order_number AS cs_order_number
 from catalog_sales group by cs_order_number
 having count(distinct cs_warehouse_sk) > 1)
  and cs1.cs_order_number not in (select cr_order_number from catalog_returns)
order by ((`order count`) IS NULL), `order count` limit 100;

--= query_17
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  store.s_state AS s_state,
  count(store_sales.ss_quantity) AS store_sales_quantitycount,
  avg(store_sales.ss_quantity) AS store_sales_quantityave,
  (CASE WHEN Math::IsNaN(stddev_samp(store_sales.ss_quantity)) THEN NULL ELSE stddev_samp(store_sales.ss_quantity) END) AS store_sales_quantitystdev,
  (CASE WHEN Math::IsNaN(stddev_samp(store_sales.ss_quantity)) THEN NULL ELSE stddev_samp(store_sales.ss_quantity) END)/avg(store_sales.ss_quantity) AS store_sales_quantitycov,
  count(store_returns.sr_return_quantity) AS store_returns_quantitycount,
  avg(store_returns.sr_return_quantity) AS store_returns_quantityave,
  (CASE WHEN Math::IsNaN(stddev_samp(store_returns.sr_return_quantity)) THEN NULL ELSE stddev_samp(store_returns.sr_return_quantity) END) AS store_returns_quantitystdev,
  (CASE WHEN Math::IsNaN(stddev_samp(store_returns.sr_return_quantity)) THEN NULL ELSE stddev_samp(store_returns.sr_return_quantity) END)/avg(store_returns.sr_return_quantity) AS store_returns_quantitycov,
  count(catalog_sales.cs_quantity) AS catalog_sales_quantitycount,
  avg(catalog_sales.cs_quantity) AS catalog_sales_quantityave,
  (CASE WHEN Math::IsNaN(stddev_samp(catalog_sales.cs_quantity)) THEN NULL ELSE stddev_samp(catalog_sales.cs_quantity) END) AS catalog_sales_quantitystdev,
  (CASE WHEN Math::IsNaN(stddev_samp(catalog_sales.cs_quantity)) THEN NULL ELSE stddev_samp(catalog_sales.cs_quantity) END)/avg(catalog_sales.cs_quantity) AS catalog_sales_quantitycov
 from store_sales
     ,store_returns
     ,catalog_sales
     ,date_dim d1
     ,date_dim d2
     ,date_dim d3
     ,store
     ,item
 where d1.d_quarter_name = '2001Q1'
   and d1.d_date_sk = ss_sold_date_sk
   and i_item_sk = ss_item_sk
   and s_store_sk = ss_store_sk
   and ss_customer_sk = sr_customer_sk
   and ss_item_sk = sr_item_sk
   and ss_ticket_number = sr_ticket_number
   and sr_returned_date_sk = d2.d_date_sk
   and d2.d_quarter_name in ('2001Q1','2001Q2','2001Q3')
   and sr_customer_sk = cs_bill_customer_sk
   and sr_item_sk = cs_item_sk
   and cs_sold_date_sk = d3.d_date_sk
   and d3.d_quarter_name in ('2001Q1','2001Q2','2001Q3')
 group by item.i_item_id, item.i_item_desc, store.s_state
 order by ((i_item_id) IS NULL), i_item_id, ((i_item_desc) IS NULL), i_item_desc, ((s_state) IS NULL), s_state limit 100;

--= query_18
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  customer_address.ca_country AS ca_country,
  customer_address.ca_state AS ca_state,
  customer_address.ca_county AS ca_county,
  avg( cast(catalog_sales.cs_quantity as Double)) AS agg1,
  avg( cast(catalog_sales.cs_list_price as Double)) AS agg2,
  avg( cast(catalog_sales.cs_coupon_amt as Double)) AS agg3,
  avg( cast(catalog_sales.cs_sales_price as Double)) AS agg4,
  avg( cast(catalog_sales.cs_net_profit as Double)) AS agg5,
  avg( cast(customer.c_birth_year as Double)) AS agg6,
  avg( cast(cd1.cd_dep_count as Double)) AS agg7
 from catalog_sales, customer_demographics cd1,
      customer_demographics cd2, customer, customer_address, date_dim, item
 where cs_sold_date_sk = d_date_sk and
       cs_item_sk = i_item_sk and
       cs_bill_cdemo_sk = cd1.cd_demo_sk and
       cs_bill_customer_sk = c_customer_sk and
       cd1.cd_gender = 'F' and
       cd1.cd_education_status = 'Unknown' and
       c_current_cdemo_sk = cd2.cd_demo_sk and
       c_current_addr_sk = ca_address_sk and
       c_birth_month in (1,6,8,9,12,2) and
       d_year = 1998 and
       ca_state in ('AL','MO','TN'
                   ,'GA','MT','IN','CA')
 group by  rollup (item.i_item_id, customer_address.ca_country, customer_address.ca_state, customer_address.ca_county)

 order by ((ca_country) IS NULL), ca_country, ((ca_state) IS NULL), ca_state, ((ca_county) IS NULL), ca_county, ((i_item_id) IS NULL), i_item_id limit 100;

--= query_19
PRAGMA AnsiImplicitCrossJoin;
select item.i_brand_id AS brand_id,
  item.i_brand AS brand,
  item.i_manufact_id AS i_manufact_id,
  item.i_manufact AS i_manufact,
  sum(store_sales.ss_ext_sales_price) AS ext_price
 from date_dim, store_sales, item,customer,customer_address,store
 where d_date_sk = ss_sold_date_sk
   and ss_item_sk = i_item_sk
   and i_manager_id=8
   and d_moy=11
   and d_year=1998
   and ss_customer_sk = c_customer_sk
   and c_current_addr_sk = ca_address_sk
   and Unicode::Substring(ca_zip, CAST(0 AS Uint32), CAST(5 AS Uint32)) <> Unicode::Substring(s_zip, CAST(0 AS Uint32), CAST(5 AS Uint32))
   and ss_store_sk = s_store_sk
 group by item.i_brand, item.i_brand_id, item.i_manufact_id, item.i_manufact
 order by ((ext_price) IS NOT NULL), ext_price DESC, ((brand) IS NULL), brand, ((brand_id) IS NULL), brand_id, ((i_manufact_id) IS NULL), i_manufact_id, ((i_manufact) IS NULL), i_manufact limit 100;

--= query_20
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  item.i_category AS i_category,
  item.i_class AS i_class,
  item.i_current_price AS i_current_price,
  sum(catalog_sales.cs_ext_sales_price) AS itemrevenue,
  sum(catalog_sales.cs_ext_sales_price)*100/sum(sum(catalog_sales.cs_ext_sales_price)) over
           (partition by item.i_class) AS revenueratio
 from   catalog_sales
     ,item
     ,date_dim
 where cs_item_sk = i_item_sk
   and i_category in ('Books', 'Music', 'Sports')
   and cs_sold_date_sk = d_date_sk
 and d_date between ('1999-02-22')
                and '1999-03-24'
 group by item.i_item_id, item.i_item_desc, item.i_category, item.i_class, item.i_current_price
 order by ((i_category) IS NULL), i_category, ((i_class) IS NULL), i_class, ((i_item_id) IS NULL), i_item_id, ((i_item_desc) IS NULL), i_item_desc, ((revenueratio) IS NULL), revenueratio limit 100;

--= query_21
PRAGMA AnsiImplicitCrossJoin;
select w_warehouse_name, i_item_id, inv_before, inv_after from(select warehouse.w_warehouse_name AS w_warehouse_name,
  item.i_item_id AS i_item_id,
  sum(case when ((date_dim.d_date) < ('2000-03-11'))
                    then inventory.inv_quantity_on_hand
                      else 0 end) AS inv_before,
  sum(case when ((date_dim.d_date) >= ('2000-03-11'))
                      then inventory.inv_quantity_on_hand
                      else 0 end) AS inv_after
 from inventory
       ,warehouse
       ,item
       ,date_dim
   where i_current_price between 0.99 and 1.49
     and i_item_sk          = inv_item_sk
     and inv_warehouse_sk   = w_warehouse_sk
     and inv_date_sk    = d_date_sk
     and d_date between '2000-02-10'
                    and '2000-04-10'
   group by warehouse.w_warehouse_name, item.i_item_id
 ) x
 where (case when inv_before > 0
             then inv_after / inv_before
             else null
             end) between 2.0/3.0 and 3.0/2.0
 order by ((w_warehouse_name) IS NULL), w_warehouse_name, ((i_item_id) IS NULL), i_item_id limit 100;

--= query_22
PRAGMA AnsiImplicitCrossJoin;
select item.i_product_name AS i_product_name,
  item.i_brand AS i_brand,
  item.i_class AS i_class,
  item.i_category AS i_category,
  avg(inventory.inv_quantity_on_hand) AS qoh
 from inventory
           ,date_dim
           ,item
       where inv_date_sk=d_date_sk
              and inv_item_sk=i_item_sk
              and d_month_seq between 1200 and 1200 + 11
       group by  rollup(item.i_product_name
                       ,item.i_brand
                       ,item.i_class
                       ,item.i_category)

 order by ((qoh) IS NULL), qoh, ((i_product_name) IS NULL), i_product_name, ((i_brand) IS NULL), i_brand, ((i_class) IS NULL), i_class, ((i_category) IS NULL), i_category limit 100;

--= query_23_a
PRAGMA AnsiImplicitCrossJoin;
$frequent_ss_items = (select itemdesc,
  item.i_item_sk AS item_sk,
  date_dim.d_date AS solddate,
  count(*) AS cnt
 from store_sales
      ,date_dim
      ,item
  where ss_sold_date_sk = d_date_sk
    and ss_item_sk = i_item_sk
    and d_year in (2000,2000+1,2000+2,2000+3)
  group by Unicode::Substring(item.i_item_desc, CAST(0 AS Uint32), CAST(30 AS Uint32)) AS itemdesc, item.i_item_sk, date_dim.d_date
 having count(*) >4);
$max_store_sales = (select max(csales) tpcds_cmax
  from (select customer.c_customer_sk AS c_customer_sk,
  sum(store_sales.ss_quantity*store_sales.ss_sales_price) AS csales
 from store_sales
            ,customer
            ,date_dim
        where ss_customer_sk = c_customer_sk
         and ss_sold_date_sk = d_date_sk
         and d_year in (2000,2000+1,2000+2,2000+3)
        group by customer.c_customer_sk
 ) AS _sq1);
$best_ss_customer = (select customer.c_customer_sk AS c_customer_sk,
  sum(store_sales.ss_quantity*store_sales.ss_sales_price) AS ssales
 from store_sales
      ,customer
  where ss_customer_sk = c_customer_sk
  group by customer.c_customer_sk
 having sum(store_sales.ss_quantity*store_sales.ss_sales_price) > (50/100.0) * (select
  *
from
 $max_store_sales AS max_store_sales));

  select  sum(sales)
 from (select cs_quantity*cs_list_price sales
       from catalog_sales
           ,date_dim
       where d_year = 2000
         and d_moy = 2
         and cs_sold_date_sk = d_date_sk
         and cs_item_sk in (select item_sk from $frequent_ss_items AS frequent_ss_items)
         and cs_bill_customer_sk in (select c_customer_sk from $best_ss_customer AS best_ss_customer)
      union all
      select ws_quantity*ws_list_price sales
       from web_sales
           ,date_dim
       where d_year = 2000
         and d_moy = 2
         and ws_sold_date_sk = d_date_sk
         and ws_item_sk in (select item_sk from $frequent_ss_items AS frequent_ss_items)
         and ws_bill_customer_sk in (select c_customer_sk from $best_ss_customer AS best_ss_customer)) AS _sq2
 limit 100;

--= query_23_b
PRAGMA AnsiImplicitCrossJoin;
$frequent_ss_items = (select itemdesc,
  item.i_item_sk AS item_sk,
  date_dim.d_date AS solddate,
  count(*) AS cnt
 from store_sales
      ,date_dim
      ,item
  where ss_sold_date_sk = d_date_sk
    and ss_item_sk = i_item_sk
    and d_year in (2000,2000 + 1,2000 + 2,2000 + 3)
  group by Unicode::Substring(item.i_item_desc, CAST(0 AS Uint32), CAST(30 AS Uint32)) AS itemdesc, item.i_item_sk, date_dim.d_date
 having count(*) >4);
$max_store_sales = (select max(csales) tpcds_cmax
  from (select customer.c_customer_sk AS c_customer_sk,
  sum(store_sales.ss_quantity*store_sales.ss_sales_price) AS csales
 from store_sales
            ,customer
            ,date_dim
        where ss_customer_sk = c_customer_sk
         and ss_sold_date_sk = d_date_sk
         and d_year in (2000,2000+1,2000+2,2000+3)
        group by customer.c_customer_sk
 ) AS _sq1);
$best_ss_customer = (select customer.c_customer_sk AS c_customer_sk,
  sum(store_sales.ss_quantity*store_sales.ss_sales_price) AS ssales
 from store_sales
      ,customer
  where ss_customer_sk = c_customer_sk
  group by customer.c_customer_sk
 having sum(store_sales.ss_quantity*store_sales.ss_sales_price) > (50/100.0) * (select
  *
 from $max_store_sales AS max_store_sales));

  select  c_last_name,c_first_name,sales
 from (select customer.c_last_name AS c_last_name,
  customer.c_first_name AS c_first_name,
  sum(catalog_sales.cs_quantity*catalog_sales.cs_list_price) AS sales
 from catalog_sales
            ,customer
            ,date_dim
        where d_year = 2000
         and d_moy = 2
         and cs_sold_date_sk = d_date_sk
         and cs_item_sk in (select item_sk from $frequent_ss_items AS frequent_ss_items)
         and cs_bill_customer_sk in (select c_customer_sk from $best_ss_customer AS best_ss_customer)
         and cs_bill_customer_sk = c_customer_sk
       group by customer.c_last_name, customer.c_first_name
 union all
      select customer.c_last_name AS c_last_name,
  customer.c_first_name AS c_first_name,
  sum(web_sales.ws_quantity*web_sales.ws_list_price) AS sales
 from web_sales
           ,customer
           ,date_dim
       where d_year = 2000
         and d_moy = 2
         and ws_sold_date_sk = d_date_sk
         and ws_item_sk in (select item_sk from $frequent_ss_items AS frequent_ss_items)
         and ws_bill_customer_sk in (select c_customer_sk from $best_ss_customer AS best_ss_customer)
         and ws_bill_customer_sk = c_customer_sk
       group by customer.c_last_name, customer.c_first_name
 ) AS _sq2
     order by ((c_last_name) IS NULL), c_last_name, ((c_first_name) IS NULL), c_first_name, ((sales) IS NULL), sales limit 100;

--= query_24_a
PRAGMA AnsiImplicitCrossJoin;
$ssales = (select customer.c_last_name AS c_last_name,
  customer.c_first_name AS c_first_name,
  store.s_store_name AS s_store_name,
  customer_address.ca_state AS ca_state,
  store.s_state AS s_state,
  item.i_color AS i_color,
  item.i_current_price AS i_current_price,
  item.i_manager_id AS i_manager_id,
  item.i_units AS i_units,
  item.i_size AS i_size,
  sum(store_sales.ss_net_paid) AS netpaid
 from store_sales
    ,store_returns
    ,store
    ,item
    ,customer
    ,customer_address
where ss_ticket_number = sr_ticket_number
  and ss_item_sk = sr_item_sk
  and ss_customer_sk = c_customer_sk
  and ss_item_sk = i_item_sk
  and ss_store_sk = s_store_sk
  and c_current_addr_sk = ca_address_sk
  and c_birth_country <> Unicode::ToUpper(ca_country)
  and s_zip = ca_zip
and s_market_id=8
group by customer.c_last_name, customer.c_first_name, store.s_store_name, customer_address.ca_state, store.s_state, item.i_color, item.i_current_price, item.i_manager_id, item.i_units, item.i_size
 );

select c_last_name AS c_last_name,
  c_first_name AS c_first_name,
  s_store_name AS s_store_name,
  sum(netpaid) AS paid
 from $ssales AS ssales
where i_color = 'peach'
group by c_last_name, c_first_name, s_store_name
 having sum(netpaid) > (select 0.05*avg(netpaid)
                                 from $ssales AS ssales)
order by ((c_last_name) IS NULL), c_last_name, ((c_first_name) IS NULL), c_first_name, ((s_store_name) IS NULL), s_store_name;

--= query_24_b
PRAGMA AnsiImplicitCrossJoin;
$ssales = (select customer.c_last_name AS c_last_name,
  customer.c_first_name AS c_first_name,
  store.s_store_name AS s_store_name,
  customer_address.ca_state AS ca_state,
  store.s_state AS s_state,
  item.i_color AS i_color,
  item.i_current_price AS i_current_price,
  item.i_manager_id AS i_manager_id,
  item.i_units AS i_units,
  item.i_size AS i_size,
  sum(store_sales.ss_net_paid) AS netpaid
 from store_sales
    ,store_returns
    ,store
    ,item
    ,customer
    ,customer_address
where ss_ticket_number = sr_ticket_number
  and ss_item_sk = sr_item_sk
  and ss_customer_sk = c_customer_sk
  and ss_item_sk = i_item_sk
  and ss_store_sk = s_store_sk
  and c_current_addr_sk = ca_address_sk
  and c_birth_country <> Unicode::ToUpper(ca_country)
  and s_zip = ca_zip
  and s_market_id = 8
group by customer.c_last_name, customer.c_first_name, store.s_store_name, customer_address.ca_state, store.s_state, item.i_color, item.i_current_price, item.i_manager_id, item.i_units, item.i_size
 );

select c_last_name AS c_last_name,
  c_first_name AS c_first_name,
  s_store_name AS s_store_name,
  sum(netpaid) AS paid
 from $ssales AS ssales
where i_color = 'saddle'
group by c_last_name, c_first_name, s_store_name
 having sum(netpaid) > (select 0.05*avg(netpaid)
                           from $ssales AS ssales)
order by ((c_last_name) IS NULL), c_last_name, ((c_first_name) IS NULL), c_first_name, ((s_store_name) IS NULL), s_store_name;

--= query_25
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  store.s_store_id AS s_store_id,
  store.s_store_name AS s_store_name,
  sum(store_sales.ss_net_profit) AS store_sales_profit,
  sum(store_returns.sr_net_loss) AS store_returns_loss,
  sum(catalog_sales.cs_net_profit) AS catalog_sales_profit
 from
 store_sales
 ,store_returns
 ,catalog_sales
 ,date_dim d1
 ,date_dim d2
 ,date_dim d3
 ,store
 ,item
 where
 d1.d_moy = 4
 and d1.d_year = 2001
 and d1.d_date_sk = ss_sold_date_sk
 and i_item_sk = ss_item_sk
 and s_store_sk = ss_store_sk
 and ss_customer_sk = sr_customer_sk
 and ss_item_sk = sr_item_sk
 and ss_ticket_number = sr_ticket_number
 and sr_returned_date_sk = d2.d_date_sk
 and d2.d_moy               between 4 and  10
 and d2.d_year              = 2001
 and sr_customer_sk = cs_bill_customer_sk
 and sr_item_sk = cs_item_sk
 and cs_sold_date_sk = d3.d_date_sk
 and d3.d_moy               between 4 and  10
 and d3.d_year              = 2001
 group by item.i_item_id, item.i_item_desc, store.s_store_id, store.s_store_name
 order by ((i_item_id) IS NULL), i_item_id, ((i_item_desc) IS NULL), i_item_desc, ((s_store_id) IS NULL), s_store_id, ((s_store_name) IS NULL), s_store_name limit 100;

--= query_26
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  avg(catalog_sales.cs_quantity) AS agg1,
  avg(catalog_sales.cs_list_price) AS agg2,
  avg(catalog_sales.cs_coupon_amt) AS agg3,
  avg(catalog_sales.cs_sales_price) AS agg4
 from catalog_sales, customer_demographics, date_dim, item, promotion
 where cs_sold_date_sk = d_date_sk and
       cs_item_sk = i_item_sk and
       cs_bill_cdemo_sk = cd_demo_sk and
       cs_promo_sk = p_promo_sk and
       cd_gender = 'M' and
       cd_marital_status = 'S' and
       cd_education_status = 'College' and
       (p_channel_email = 'N' or p_channel_event = 'N') and
       d_year = 2000
 group by item.i_item_id
 order by ((i_item_id) IS NULL), i_item_id limit 100;

--= query_27
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  store.s_state AS s_state,
  grouping(store.s_state) AS g_state,
  avg(store_sales.ss_quantity) AS agg1,
  avg(store_sales.ss_list_price) AS agg2,
  avg(store_sales.ss_coupon_amt) AS agg3,
  avg(store_sales.ss_sales_price) AS agg4
 from store_sales, customer_demographics, date_dim, store, item
 where ss_sold_date_sk = d_date_sk and
       ss_item_sk = i_item_sk and
       ss_store_sk = s_store_sk and
       ss_cdemo_sk = cd_demo_sk and
       cd_gender = 'M' and
       cd_marital_status = 'S' and
       cd_education_status = 'College' and
       d_year = 2002 and
       s_state in ('TN','TN', 'TN', 'TN', 'TN', 'TN')
 group by  rollup (item.i_item_id, store.s_state)

 order by ((i_item_id) IS NULL), i_item_id, ((s_state) IS NULL), s_state limit 100;

--= query_28
PRAGMA AnsiImplicitCrossJoin;
select B1_LP, B1_CNT, B1_CNTD, B2_LP, B2_CNT, B2_CNTD, B3_LP, B3_CNT, B3_CNTD, B4_LP, B4_CNT, B4_CNTD, B5_LP, B5_CNT, B5_CNTD, B6_LP, B6_CNT, B6_CNTD from (select avg(ss_list_price) B1_LP
            ,count(ss_list_price) B1_CNT
            ,count(distinct ss_list_price) B1_CNTD
      from store_sales
      where ss_quantity between 0 and 5
        and (ss_list_price between 8 and 8+10
             or ss_coupon_amt between 459 and 459+1000
             or ss_wholesale_cost between 57 and 57+20)) B1,
     (select avg(ss_list_price) B2_LP
            ,count(ss_list_price) B2_CNT
            ,count(distinct ss_list_price) B2_CNTD
      from store_sales
      where ss_quantity between 6 and 10
        and (ss_list_price between 90 and 90+10
          or ss_coupon_amt between 2323 and 2323+1000
          or ss_wholesale_cost between 31 and 31+20)) B2,
     (select avg(ss_list_price) B3_LP
            ,count(ss_list_price) B3_CNT
            ,count(distinct ss_list_price) B3_CNTD
      from store_sales
      where ss_quantity between 11 and 15
        and (ss_list_price between 142 and 142+10
          or ss_coupon_amt between 12214 and 12214+1000
          or ss_wholesale_cost between 79 and 79+20)) B3,
     (select avg(ss_list_price) B4_LP
            ,count(ss_list_price) B4_CNT
            ,count(distinct ss_list_price) B4_CNTD
      from store_sales
      where ss_quantity between 16 and 20
        and (ss_list_price between 135 and 135+10
          or ss_coupon_amt between 6071 and 6071+1000
          or ss_wholesale_cost between 38 and 38+20)) B4,
     (select avg(ss_list_price) B5_LP
            ,count(ss_list_price) B5_CNT
            ,count(distinct ss_list_price) B5_CNTD
      from store_sales
      where ss_quantity between 21 and 25
        and (ss_list_price between 122 and 122+10
          or ss_coupon_amt between 836 and 836+1000
          or ss_wholesale_cost between 17 and 17+20)) B5,
     (select avg(ss_list_price) B6_LP
            ,count(ss_list_price) B6_CNT
            ,count(distinct ss_list_price) B6_CNTD
      from store_sales
      where ss_quantity between 26 and 30
        and (ss_list_price between 154 and 154+10
          or ss_coupon_amt between 7326 and 7326+1000
          or ss_wholesale_cost between 7 and 7+20)) B6
limit 100;

--= query_29
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  store.s_store_id AS s_store_id,
  store.s_store_name AS s_store_name,
  sum(store_sales.ss_quantity) AS store_sales_quantity,
  sum(store_returns.sr_return_quantity) AS store_returns_quantity,
  sum(catalog_sales.cs_quantity) AS catalog_sales_quantity
 from
    store_sales
   ,store_returns
   ,catalog_sales
   ,date_dim             d1
   ,date_dim             d2
   ,date_dim             d3
   ,store
   ,item
 where
     d1.d_moy               = 9
 and d1.d_year              = 1999
 and d1.d_date_sk           = ss_sold_date_sk
 and i_item_sk              = ss_item_sk
 and s_store_sk             = ss_store_sk
 and ss_customer_sk         = sr_customer_sk
 and ss_item_sk             = sr_item_sk
 and ss_ticket_number       = sr_ticket_number
 and sr_returned_date_sk    = d2.d_date_sk
 and d2.d_moy               between 9 and  9 + 3
 and d2.d_year              = 1999
 and sr_customer_sk         = cs_bill_customer_sk
 and sr_item_sk             = cs_item_sk
 and cs_sold_date_sk        = d3.d_date_sk
 and d3.d_year              in (1999,1999+1,1999+2)
 group by item.i_item_id, item.i_item_desc, store.s_store_id, store.s_store_name
 order by ((i_item_id) IS NULL), i_item_id, ((i_item_desc) IS NULL), i_item_desc, ((s_store_id) IS NULL), s_store_id, ((s_store_name) IS NULL), s_store_name limit 100;

--= query_30
PRAGMA AnsiImplicitCrossJoin;
$customer_total_return = (select web_returns.wr_returning_customer_sk AS ctr_customer_sk,
  customer_address.ca_state AS ctr_state,
  sum(web_returns.wr_return_amt) AS ctr_total_return
 from web_returns
     ,date_dim
     ,customer_address
 where wr_returned_date_sk = d_date_sk
   and d_year =2002
   and wr_returning_addr_sk = ca_address_sk
 group by web_returns.wr_returning_customer_sk, customer_address.ca_state
 );
$ctr_state_avg = (select ctr_state AS ctr_state,
  avg(ctr_total_return)*1.2 AS ctr_avg
 from $customer_total_return AS customer_total_return group by ctr_state
 );

  select  c_customer_id,c_salutation,c_first_name,c_last_name,c_preferred_cust_flag
       ,c_birth_day,c_birth_month,c_birth_year,c_birth_country,c_login,c_email_address
       ,c_last_review_date,ctr_total_return
 from $customer_total_return AS ctr1
     ,customer_address
     ,customer
     ,$ctr_state_avg AS ctr_state_avg
 where ctr1.ctr_total_return > ctr_state_avg.ctr_avg
              and ctr_state_avg.ctr_state = ctr1.ctr_state
       and ca_address_sk = c_current_addr_sk
       and ca_state = 'GA'
       and ctr1.ctr_customer_sk = c_customer_sk
 order by ((c_customer_id) IS NULL), c_customer_id, ((c_salutation) IS NULL), c_salutation, ((c_first_name) IS NULL), c_first_name, ((c_last_name) IS NULL), c_last_name, ((c_preferred_cust_flag) IS NULL), c_preferred_cust_flag, ((c_birth_day) IS NULL), c_birth_day, ((c_birth_month) IS NULL), c_birth_month, ((c_birth_year) IS NULL), c_birth_year, ((c_birth_country) IS NULL), c_birth_country, ((c_login) IS NULL), c_login, ((c_email_address) IS NULL), c_email_address, ((c_last_review_date) IS NULL), c_last_review_date, ((ctr_total_return) IS NULL), ctr_total_return limit 100;

--= query_31
PRAGMA AnsiImplicitCrossJoin;
$ss = (select customer_address.ca_county AS ca_county,
  date_dim.d_qoy AS d_qoy,
  date_dim.d_year AS d_year,
  sum(store_sales.ss_ext_sales_price) AS store_sales
 from store_sales,date_dim,customer_address
 where ss_sold_date_sk = d_date_sk
  and ss_addr_sk=ca_address_sk
 group by customer_address.ca_county, date_dim.d_qoy, date_dim.d_year
 );
$ws = (select customer_address.ca_county AS ca_county,
  date_dim.d_qoy AS d_qoy,
  date_dim.d_year AS d_year,
  sum(web_sales.ws_ext_sales_price) AS web_sales
 from web_sales,date_dim,customer_address
 where ws_sold_date_sk = d_date_sk
  and ws_bill_addr_sk=ca_address_sk
 group by customer_address.ca_county, date_dim.d_qoy, date_dim.d_year
 );

 select
        ss1.ca_county AS ca_county
       ,ss1.d_year AS d_year
       ,ws2.web_sales/ws1.web_sales web_q1_q2_increase
       ,ss2.store_sales/ss1.store_sales store_q1_q2_increase
       ,ws3.web_sales/ws2.web_sales web_q2_q3_increase
       ,ss3.store_sales/ss2.store_sales store_q2_q3_increase
 from
        $ss AS ss1
       ,$ss AS ss2
       ,$ss AS ss3
       ,$ws AS ws1
       ,$ws AS ws2
       ,$ws AS ws3
 where
    ss1.d_qoy = 1
    and ss1.d_year = 2000
    and ss1.ca_county = ss2.ca_county
    and ss2.d_qoy = 2
    and ss2.d_year = 2000
 and ss2.ca_county = ss3.ca_county
    and ss3.d_qoy = 3
    and ss3.d_year = 2000
    and ss1.ca_county = ws1.ca_county
    and ws1.d_qoy = 1
    and ws1.d_year = 2000
    and ws1.ca_county = ws2.ca_county
    and ws2.d_qoy = 2
    and ws2.d_year = 2000
    and ws1.ca_county = ws3.ca_county
    and ws3.d_qoy = 3
    and ws3.d_year =2000
    and case when ws1.web_sales > 0 then ws2.web_sales/ws1.web_sales else null end
       > case when ss1.store_sales > 0 then ss2.store_sales/ss1.store_sales else null end
    and case when ws2.web_sales > 0 then ws3.web_sales/ws2.web_sales else null end
       > case when ss2.store_sales > 0 then ss3.store_sales/ss2.store_sales else null end
 order by ((store_q2_q3_increase) IS NULL), store_q2_q3_increase;

--= query_32
PRAGMA AnsiImplicitCrossJoin;
$disc = (select catalog_sales.cs_item_sk AS ditem,
  1.3 * avg(catalog_sales.cs_ext_discount_amt) AS avg_disc
 from catalog_sales, date_dim
  where d_date between '2000-01-27' and '2000-04-26'
    and d_date_sk = cs_sold_date_sk
  group by catalog_sales.cs_item_sk
 );

select sum(catalog_sales.cs_ext_discount_amt) AS `excess discount amount`
 from catalog_sales, item, date_dim, $disc AS disc
where i_manufact_id = 977
  and i_item_sk = cs_item_sk
  and d_date between '2000-01-27' and '2000-04-26'
  and d_date_sk = cs_sold_date_sk
  and disc.ditem = catalog_sales.cs_item_sk
  and catalog_sales.cs_ext_discount_amt > disc.avg_disc
limit 100;

--= query_33
PRAGMA AnsiImplicitCrossJoin;
$ss = (select item.i_manufact_id AS i_manufact_id,
  sum(store_sales.ss_ext_sales_price) AS total_sales
 from
    store_sales,
    date_dim,
         customer_address,
         item
 where
         i_manufact_id in (select
  i_manufact_id
from
 item
where i_category in ('Electronics'))
 and     ss_item_sk              = i_item_sk
 and     ss_sold_date_sk         = d_date_sk
 and     d_year                  = 1998
 and     d_moy                   = 5
 and     ss_addr_sk              = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_manufact_id
 );
$cs = (select item.i_manufact_id AS i_manufact_id,
  sum(catalog_sales.cs_ext_sales_price) AS total_sales
 from
    catalog_sales,
    date_dim,
         customer_address,
         item
 where
         i_manufact_id               in (select
  i_manufact_id
from
 item
where i_category in ('Electronics'))
 and     cs_item_sk              = i_item_sk
 and     cs_sold_date_sk         = d_date_sk
 and     d_year                  = 1998
 and     d_moy                   = 5
 and     cs_bill_addr_sk         = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_manufact_id
 );
$ws = (select item.i_manufact_id AS i_manufact_id,
  sum(web_sales.ws_ext_sales_price) AS total_sales
 from
    web_sales,
    date_dim,
         customer_address,
         item
 where
         i_manufact_id               in (select
  i_manufact_id
from
 item
where i_category in ('Electronics'))
 and     ws_item_sk              = i_item_sk
 and     ws_sold_date_sk         = d_date_sk
 and     d_year                  = 1998
 and     d_moy                   = 5
 and     ws_bill_addr_sk         = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_manufact_id
 );

  select i_manufact_id AS i_manufact_id,
  sum(total_sales) AS total_sales
 from  (select * from $ss AS ss
        union all
        select * from $cs AS cs
        union all
        select * from $ws AS ws) tmp1
 group by i_manufact_id
 order by ((total_sales) IS NULL), total_sales limit 100;

--= query_34
PRAGMA AnsiImplicitCrossJoin;
select c_last_name
       ,c_first_name
       ,c_salutation
       ,c_preferred_cust_flag
       ,ss_ticket_number
       ,cnt from
   (select store_sales.ss_ticket_number AS ss_ticket_number,
  store_sales.ss_customer_sk AS ss_customer_sk,
  count(*) AS cnt
 from store_sales,date_dim,store,household_demographics
    where store_sales.ss_sold_date_sk = date_dim.d_date_sk
    and store_sales.ss_store_sk = store.s_store_sk
    and store_sales.ss_hdemo_sk = household_demographics.hd_demo_sk
    and (date_dim.d_dom between 1 and 3 or date_dim.d_dom between 25 and 28)
    and (household_demographics.hd_buy_potential = '>10000' or
         household_demographics.hd_buy_potential = 'Unknown')
    and household_demographics.hd_vehicle_count > 0
    and (case when household_demographics.hd_vehicle_count > 0
    then household_demographics.hd_dep_count/ household_demographics.hd_vehicle_count
    else null
    end)  > 1.2
    and date_dim.d_year in (1999,1999+1,1999+2)
    and store.s_county in ('Williamson County','Williamson County','Williamson County','Williamson County',
                           'Williamson County','Williamson County','Williamson County','Williamson County')
    group by store_sales.ss_ticket_number, store_sales.ss_customer_sk
 ) dn,customer
    where ss_customer_sk = c_customer_sk
      and cnt between 15 and 20
    order by ((c_last_name) IS NULL), c_last_name, ((c_first_name) IS NULL), c_first_name, ((c_salutation) IS NULL), c_salutation, ((c_preferred_cust_flag) IS NOT NULL), c_preferred_cust_flag DESC, ((ss_ticket_number) IS NULL), ss_ticket_number;

--= query_35
PRAGMA AnsiImplicitCrossJoin;
select ca.ca_state AS ca_state,
  customer_demographics.cd_gender AS cd_gender,
  customer_demographics.cd_marital_status AS cd_marital_status,
  customer_demographics.cd_dep_count AS cd_dep_count,
  count(*) AS cnt1,
  min(customer_demographics.cd_dep_count) AS aggone1,
  (CASE WHEN Math::IsNaN(max(customer_demographics.cd_dep_count)) THEN NULL ELSE stddev_samp(customer_demographics.cd_dep_count) END) AS aggtwo1,
  avg(customer_demographics.cd_dep_count) AS aggthree1,
  customer_demographics.cd_dep_employed_count AS cd_dep_employed_count,
  count(*) AS cnt2,
  min(customer_demographics.cd_dep_employed_count) AS aggone2,
  (CASE WHEN Math::IsNaN(max(customer_demographics.cd_dep_employed_count)) THEN NULL ELSE stddev_samp(customer_demographics.cd_dep_employed_count) END) AS aggtwo2,
  avg(customer_demographics.cd_dep_employed_count) AS aggthree2,
  customer_demographics.cd_dep_college_count AS cd_dep_college_count,
  count(*) AS cnt3,
  min(customer_demographics.cd_dep_college_count) AS aggone3,
  (CASE WHEN Math::IsNaN(max(customer_demographics.cd_dep_college_count)) THEN NULL ELSE stddev_samp(customer_demographics.cd_dep_college_count) END) AS aggtwo3,
  avg(customer_demographics.cd_dep_college_count) AS aggthree3
 from
  customer c,customer_address ca,customer_demographics
 where
  c.c_current_addr_sk = ca.ca_address_sk and
  cd_demo_sk = c.c_current_cdemo_sk and
  c.c_customer_sk in (select ss_customer_sk from store_sales,date_dim where ss_sold_date_sk = d_date_sk and
                d_year = 2002 and
                d_qoy < 4) and
   (c.c_customer_sk in (select ws_bill_customer_sk from web_sales,date_dim where ws_sold_date_sk = d_date_sk and
                  d_year = 2002 and
                  d_qoy < 4) or
    c.c_customer_sk in (select cs_ship_customer_sk from catalog_sales,date_dim where cs_sold_date_sk = d_date_sk and
                  d_year = 2002 and
                  d_qoy < 4))
 group by ca.ca_state, customer_demographics.cd_gender, customer_demographics.cd_marital_status, customer_demographics.cd_dep_count, customer_demographics.cd_dep_employed_count, customer_demographics.cd_dep_college_count
 order by ((ca_state) IS NULL), ca_state, ((cd_gender) IS NULL), cd_gender, ((cd_marital_status) IS NULL), cd_marital_status, ((cd_dep_count) IS NULL), cd_dep_count, ((cd_dep_employed_count) IS NULL), cd_dep_employed_count, ((cd_dep_college_count) IS NULL), cd_dep_college_count limit 100;

--= query_36
PRAGMA AnsiImplicitCrossJoin;
select sum(store_sales.ss_net_profit)/sum(store_sales.ss_ext_sales_price) AS gross_margin,
  item.i_category AS i_category,
  item.i_class AS i_class,
  grouping(item.i_category)+grouping(item.i_class) AS lochierarchy,
  rank() over (
    partition by grouping(item.i_category)+grouping(item.i_class),
    case when grouping(item.i_class) = 0 then item.i_category  else null end
    order by ((sum(store_sales.ss_net_profit)/sum(store_sales.ss_ext_sales_price)) IS NULL), sum(store_sales.ss_net_profit)/sum(store_sales.ss_ext_sales_price) ASC ) AS rank_within_parent
 from
    store_sales
   ,date_dim       d1
   ,item
   ,store
 where
    d1.d_year = 2001
 and d1.d_date_sk = ss_sold_date_sk
 and i_item_sk  = ss_item_sk
 and s_store_sk  = ss_store_sk
 and s_state in ('TN','TN','TN','TN',
                 'TN','TN','TN','TN')
 group by  rollup(item.i_category,item.i_class)

 order by ((lochierarchy) IS NOT NULL), lochierarchy DESC, ((case when lochierarchy = 0 then i_category  else null end) IS NULL), case when lochierarchy = 0 then i_category  else null end, ((rank_within_parent) IS NULL), rank_within_parent limit 100;

--= query_37
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  item.i_current_price AS i_current_price
 from item, inventory, date_dim, catalog_sales
 where i_current_price between 68 and 68 + 30
 and inv_item_sk = i_item_sk
 and d_date_sk=inv_date_sk
 and d_date between ('2000-02-01') and '2000-04-01'
 and i_manufact_id in (677,940,694,808)
 and inv_quantity_on_hand between 100 and 500
 and cs_item_sk = i_item_sk
 group by item.i_item_id, item.i_item_desc, item.i_current_price
 order by ((i_item_id) IS NULL), i_item_id limit 100;

--= query_38
PRAGMA AnsiImplicitCrossJoin;
select  count(*) from (
    select distinct c_last_name, c_first_name, d_date
    from store_sales, date_dim, customer
          where store_sales.ss_sold_date_sk = date_dim.d_date_sk
      and store_sales.ss_customer_sk = customer.c_customer_sk
      and d_month_seq between 1200 and 1200 + 11
  intersect
    select distinct c_last_name, c_first_name, d_date
    from catalog_sales, date_dim, customer
          where catalog_sales.cs_sold_date_sk = date_dim.d_date_sk
      and catalog_sales.cs_bill_customer_sk = customer.c_customer_sk
      and d_month_seq between 1200 and 1200 + 11
  intersect
    select distinct c_last_name, c_first_name, d_date
    from web_sales, date_dim, customer
          where web_sales.ws_sold_date_sk = date_dim.d_date_sk
      and web_sales.ws_bill_customer_sk = customer.c_customer_sk
      and d_month_seq between 1200 and 1200 + 11
) hot_cust
limit 100;

--= query_39_a
PRAGMA AnsiImplicitCrossJoin;
$inv = (select w_warehouse_name,w_warehouse_sk,i_item_sk,d_moy
       ,stdev,mean, case mean when 0 then null else stdev/mean end cov
 from(select warehouse.w_warehouse_name AS w_warehouse_name,
  warehouse.w_warehouse_sk AS w_warehouse_sk,
  item.i_item_sk AS i_item_sk,
  date_dim.d_moy AS d_moy,
  (CASE WHEN Math::IsNaN(stddev_samp(inventory.inv_quantity_on_hand)) THEN NULL ELSE stddev_samp(inventory.inv_quantity_on_hand) END) AS stdev,
  avg(inventory.inv_quantity_on_hand) AS mean
 from inventory
          ,item
          ,warehouse
          ,date_dim
      where inv_item_sk = i_item_sk
        and inv_warehouse_sk = w_warehouse_sk
        and inv_date_sk = d_date_sk
        and d_year =2001
      group by warehouse.w_warehouse_name, warehouse.w_warehouse_sk, item.i_item_sk, date_dim.d_moy
 ) foo
 where case mean when 0 then 0 else stdev/mean end > 1);

select inv1.w_warehouse_sk,inv1.i_item_sk,inv1.d_moy,inv1.mean, inv1.cov
        ,inv2.w_warehouse_sk,inv2.i_item_sk,inv2.d_moy,inv2.mean, inv2.cov
from $inv AS inv1,$inv AS inv2
where inv1.i_item_sk = inv2.i_item_sk
  and inv1.w_warehouse_sk =  inv2.w_warehouse_sk
  and inv1.d_moy=1
  and inv2.d_moy=1+1
order by ((inv1.w_warehouse_sk) IS NULL), inv1.w_warehouse_sk, ((inv1.i_item_sk) IS NULL), inv1.i_item_sk, ((inv1.d_moy) IS NULL), inv1.d_moy, ((inv1.mean) IS NULL), inv1.mean, ((inv1.cov) IS NULL), inv1.cov, ((inv2.d_moy) IS NULL), inv2.d_moy, ((inv2.mean) IS NULL), inv2.mean, ((inv2.cov) IS NULL), inv2.cov;

--= query_39_b
PRAGMA AnsiImplicitCrossJoin;
$inv = (select w_warehouse_name,w_warehouse_sk,i_item_sk,d_moy
       ,stdev,mean, case mean when 0 then null else stdev/mean end cov
 from(select warehouse.w_warehouse_name AS w_warehouse_name,
  warehouse.w_warehouse_sk AS w_warehouse_sk,
  item.i_item_sk AS i_item_sk,
  date_dim.d_moy AS d_moy,
  (CASE WHEN Math::IsNaN(stddev_samp(inventory.inv_quantity_on_hand)) THEN NULL ELSE stddev_samp(inventory.inv_quantity_on_hand) END) AS stdev,
  avg(inventory.inv_quantity_on_hand) AS mean
 from inventory
          ,item
          ,warehouse
          ,date_dim
      where inv_item_sk = i_item_sk
        and inv_warehouse_sk = w_warehouse_sk
        and inv_date_sk = d_date_sk
        and d_year =2001
      group by warehouse.w_warehouse_name, warehouse.w_warehouse_sk, item.i_item_sk, date_dim.d_moy
 ) foo
 where case mean when 0 then 0 else stdev/mean end > 1);

select inv1.w_warehouse_sk,inv1.i_item_sk,inv1.d_moy,inv1.mean, inv1.cov
        ,inv2.w_warehouse_sk,inv2.i_item_sk,inv2.d_moy,inv2.mean, inv2.cov
from $inv AS inv1,$inv AS inv2
where inv1.i_item_sk = inv2.i_item_sk
  and inv1.w_warehouse_sk =  inv2.w_warehouse_sk
  and inv1.d_moy=1
  and inv2.d_moy=1+1
  and inv1.cov > 1.5
order by ((inv1.w_warehouse_sk) IS NULL), inv1.w_warehouse_sk, ((inv1.i_item_sk) IS NULL), inv1.i_item_sk, ((inv1.d_moy) IS NULL), inv1.d_moy, ((inv1.mean) IS NULL), inv1.mean, ((inv1.cov) IS NULL), inv1.cov, ((inv2.d_moy) IS NULL), inv2.d_moy, ((inv2.mean) IS NULL), inv2.mean, ((inv2.cov) IS NULL), inv2.cov;

--= query_40
PRAGMA AnsiImplicitCrossJoin;
select warehouse.w_state AS w_state,
  item.i_item_id AS i_item_id,
  sum(case when ((date_dim.d_date) < ('2000-03-11'))
        then catalog_sales.cs_sales_price - coalesce(catalog_returns.cr_refunded_cash,0) else 0 end) AS sales_before,
  sum(case when ((date_dim.d_date) >= ('2000-03-11'))
        then catalog_sales.cs_sales_price - coalesce(catalog_returns.cr_refunded_cash,0) else 0 end) AS sales_after
 from
   catalog_sales left outer join catalog_returns on
       (catalog_sales.cs_order_number = catalog_returns.cr_order_number
        and catalog_sales.cs_item_sk = catalog_returns.cr_item_sk)
  ,warehouse
  ,item
  ,date_dim
 where
     i_current_price between 0.99 and 1.49
 and i_item_sk          = cs_item_sk
 and cs_warehouse_sk    = w_warehouse_sk
 and cs_sold_date_sk    = d_date_sk
 and d_date between '2000-02-10'
                and '2000-04-10'
 group by warehouse.w_state, item.i_item_id
 order by ((w_state) IS NULL), w_state, ((i_item_id) IS NULL), i_item_id limit 100;

--= query_41
PRAGMA AnsiImplicitCrossJoin;
$mfg = (select distinct i_manufact from item
  where (((i_category = 'Women' and (i_color = 'powder' or i_color = 'khaki') and (i_units = 'Ounce' or i_units = 'Oz') and (i_size = 'medium' or i_size = 'extra large'))
       or (i_category = 'Women' and (i_color = 'brown' or i_color = 'honeydew') and (i_units = 'Bunch' or i_units = 'Ton') and (i_size = 'N/A' or i_size = 'small'))
       or (i_category = 'Men' and (i_color = 'floral' or i_color = 'deep') and (i_units = 'N/A' or i_units = 'Dozen') and (i_size = 'petite' or i_size = 'large'))
       or (i_category = 'Men' and (i_color = 'light' or i_color = 'cornflower') and (i_units = 'Box' or i_units = 'Pound') and (i_size = 'medium' or i_size = 'extra large')))
     or ((i_category = 'Women' and (i_color = 'midnight' or i_color = 'snow') and (i_units = 'Pallet' or i_units = 'Gross') and (i_size = 'medium' or i_size = 'extra large'))
       or (i_category = 'Women' and (i_color = 'cyan' or i_color = 'papaya') and (i_units = 'Cup' or i_units = 'Dram') and (i_size = 'N/A' or i_size = 'small'))
       or (i_category = 'Men' and (i_color = 'orange' or i_color = 'frosted') and (i_units = 'Each' or i_units = 'Tbl') and (i_size = 'petite' or i_size = 'large'))
       or (i_category = 'Men' and (i_color = 'forest' or i_color = 'ghost') and (i_units = 'Lb' or i_units = 'Bundle') and (i_size = 'medium' or i_size = 'extra large')))));

select distinct(i_product_name)
 from item i1
 where i_manufact_id between 738 and 738+40
   and i1.i_manufact in (select i_manufact from $mfg AS mfg)
 order by ((i_product_name) IS NULL), i_product_name limit 100;

--= query_42
PRAGMA AnsiImplicitCrossJoin;
select dt.d_year AS d_year,
  item.i_category_id AS i_category_id,
  item.i_category AS i_category,
  sum(store_sales.ss_ext_sales_price) AS c9
 from   date_dim dt
    ,store_sales
    ,item
 where dt.d_date_sk = store_sales.ss_sold_date_sk
    and store_sales.ss_item_sk = item.i_item_sk
    and item.i_manager_id = 1
    and dt.d_moy=11
    and dt.d_year=2000
 group by dt.d_year, item.i_category_id, item.i_category
 order by ((c9) IS NOT NULL), c9 DESC, ((d_year) IS NULL), d_year, ((i_category_id) IS NULL), i_category_id, ((i_category) IS NULL), i_category limit 100;

--= query_43
PRAGMA AnsiImplicitCrossJoin;
select store.s_store_name AS s_store_name,
  store.s_store_id AS s_store_id,
  sum(case when (date_dim.d_day_name='Sunday') then store_sales.ss_sales_price else null end) AS sun_sales,
  sum(case when (date_dim.d_day_name='Monday') then store_sales.ss_sales_price else null end) AS mon_sales,
  sum(case when (date_dim.d_day_name='Tuesday') then store_sales.ss_sales_price else  null end) AS tue_sales,
  sum(case when (date_dim.d_day_name='Wednesday') then store_sales.ss_sales_price else null end) AS wed_sales,
  sum(case when (date_dim.d_day_name='Thursday') then store_sales.ss_sales_price else null end) AS thu_sales,
  sum(case when (date_dim.d_day_name='Friday') then store_sales.ss_sales_price else null end) AS fri_sales,
  sum(case when (date_dim.d_day_name='Saturday') then store_sales.ss_sales_price else null end) AS sat_sales
 from date_dim, store_sales, store
 where d_date_sk = ss_sold_date_sk and
       s_store_sk = ss_store_sk and
       s_gmt_offset = -5 and
       d_year = 2000
 group by store.s_store_name, store.s_store_id
 order by ((s_store_name) IS NULL), s_store_name, ((s_store_id) IS NULL), s_store_id, ((sun_sales) IS NULL), sun_sales, ((mon_sales) IS NULL), mon_sales, ((tue_sales) IS NULL), tue_sales, ((wed_sales) IS NULL), wed_sales, ((thu_sales) IS NULL), thu_sales, ((fri_sales) IS NULL), fri_sales, ((sat_sales) IS NULL), sat_sales limit 100;

--= query_44
PRAGMA AnsiImplicitCrossJoin;
select  asceding.rnk AS rnk, i1.i_product_name best_performing, i2.i_product_name worst_performing
from(select *
     from (select item_sk,rank() over (order by ((rank_col) IS NULL), rank_col ASC ) rnk
           from (select ss_item_sk AS item_sk,
  avg(ss_net_profit) AS rank_col
 from store_sales ss1
                 where ss_store_sk = 4
                 group by ss_item_sk
 having avg(ss_net_profit) > 0.9*(select avg(ss_net_profit) AS rank_col
 from store_sales
                                                  where ss_store_sk = 4
                                                    and ss_addr_sk is null
                                                  group by ss_store_sk
 ))V1)V11
     where rnk  < 11) asceding,
    (select *
     from (select item_sk,rank() over (order by ((rank_col) IS NOT NULL), rank_col DESC ) rnk
           from (select ss_item_sk AS item_sk,
  avg(ss_net_profit) AS rank_col
 from store_sales ss1
                 where ss_store_sk = 4
                 group by ss_item_sk
 having avg(ss_net_profit) > 0.9*(select avg(ss_net_profit) AS rank_col
 from store_sales
                                                  where ss_store_sk = 4
                                                    and ss_addr_sk is null
                                                  group by ss_store_sk
 ))V2)V21
     where rnk  < 11) descending,
item i1,
item i2
where asceding.rnk = descending.rnk
  and i1.i_item_sk=asceding.item_sk
  and i2.i_item_sk=descending.item_sk
order by ((asceding.rnk) IS NULL), asceding.rnk limit 100;

--= query_45
PRAGMA AnsiImplicitCrossJoin;
select customer_address.ca_zip AS ca_zip,
  customer_address.ca_city AS ca_city,
  sum(web_sales.ws_sales_price) AS c10
 from web_sales, customer, customer_address, date_dim, item
 where ws_bill_customer_sk = c_customer_sk
    and c_current_addr_sk = ca_address_sk
    and ws_item_sk = i_item_sk
    and ( Unicode::Substring(ca_zip, CAST(0 AS Uint32), CAST(5 AS Uint32)) in ('85669', '86197','88274','83405','86475', '85392', '85460', '80348', '81792')
          or
          i_item_id in (select i_item_id
                             from item
                             where i_item_sk in (2, 3, 5, 7, 11, 13, 17, 19, 23, 29)
                             )
        )
    and ws_sold_date_sk = d_date_sk
    and d_qoy = 2 and d_year = 2001
 group by customer_address.ca_zip, customer_address.ca_city
 order by ((ca_zip) IS NULL), ca_zip, ((ca_city) IS NULL), ca_city limit 100;

--= query_46
PRAGMA AnsiImplicitCrossJoin;
select  c_last_name
       ,c_first_name
       ,ca_city
       ,bought_city
       ,ss_ticket_number
       ,amt,profit
 from
   (select store_sales.ss_ticket_number AS ss_ticket_number,
  store_sales.ss_customer_sk AS ss_customer_sk,
  customer_address.ca_city AS bought_city,
  sum(store_sales.ss_coupon_amt) AS amt,
  sum(store_sales.ss_net_profit) AS profit
 from store_sales,date_dim,store,household_demographics,customer_address
    where store_sales.ss_sold_date_sk = date_dim.d_date_sk
    and store_sales.ss_store_sk = store.s_store_sk
    and store_sales.ss_hdemo_sk = household_demographics.hd_demo_sk
    and store_sales.ss_addr_sk = customer_address.ca_address_sk
    and (household_demographics.hd_dep_count = 4 or
         household_demographics.hd_vehicle_count= 3)
    and date_dim.d_dow in (6,0)
    and date_dim.d_year in (1999,1999+1,1999+2)
    and store.s_city in ('Midway','Fairview','Fairview','Midway','Fairview')
    group by store_sales.ss_ticket_number, store_sales.ss_customer_sk, store_sales.ss_addr_sk, customer_address.ca_city
 ) dn,customer,customer_address current_addr
    where ss_customer_sk = c_customer_sk
      and customer.c_current_addr_sk = current_addr.ca_address_sk
      and current_addr.ca_city <> bought_city
  order by ((c_last_name) IS NULL), c_last_name, ((c_first_name) IS NULL), c_first_name, ((ca_city) IS NULL), ca_city, ((bought_city) IS NULL), bought_city, ((ss_ticket_number) IS NULL), ss_ticket_number limit 100;

--= query_47
PRAGMA AnsiImplicitCrossJoin;
$v1 = (select item.i_category AS i_category,
  item.i_brand AS i_brand,
  store.s_store_name AS s_store_name,
  store.s_company_name AS s_company_name,
  date_dim.d_year AS d_year,
  date_dim.d_moy AS d_moy,
  sum(store_sales.ss_sales_price) AS sum_sales,
  avg(sum(store_sales.ss_sales_price)) over
          (partition by item.i_category, item.i_brand,
                     store.s_store_name, store.s_company_name, date_dim.d_year) AS avg_monthly_sales,
  rank() over
          (partition by item.i_category, item.i_brand,
                     store.s_store_name, store.s_company_name
           order by ((date_dim.d_year) IS NULL), date_dim.d_year, ((date_dim.d_moy) IS NULL), date_dim.d_moy ) AS rn
 from item, store_sales, date_dim, store
 where ss_item_sk = i_item_sk and
       ss_sold_date_sk = d_date_sk and
       ss_store_sk = s_store_sk and
       (
         d_year = 1999 or
         ( d_year = 1999-1 and d_moy =12) or
         ( d_year = 1999+1 and d_moy =1)
       )
 group by item.i_category, item.i_brand, store.s_store_name, store.s_company_name, date_dim.d_year, date_dim.d_moy
 );
$v2 = (select v1.s_store_name AS s_store_name, v1.s_company_name AS s_company_name
        ,v1.d_year AS d_year
        ,v1.avg_monthly_sales AS avg_monthly_sales
        ,v1.sum_sales AS sum_sales, v1_lag.sum_sales psum, v1_lead.sum_sales nsum
 from $v1 AS v1, $v1 AS v1_lag, $v1 AS v1_lead
 where v1.i_category = v1_lag.i_category and
       v1.i_category = v1_lead.i_category and
       v1.i_brand = v1_lag.i_brand and
       v1.i_brand = v1_lead.i_brand and
       v1.s_store_name = v1_lag.s_store_name and
       v1.s_store_name = v1_lead.s_store_name and
       v1.s_company_name = v1_lag.s_company_name and
       v1.s_company_name = v1_lead.s_company_name and
       v1.rn = v1_lag.rn + 1 and
       v1.rn = v1_lead.rn - 1);

  select s_store_name, s_company_name, d_year, avg_monthly_sales, sum_sales, psum, nsum from $v2 AS v2
 where  d_year = 1999 and
        avg_monthly_sales > 0 and
        case when avg_monthly_sales > 0 then abs(sum_sales - avg_monthly_sales) / avg_monthly_sales else null end > 0.1
 order by ((sum_sales - avg_monthly_sales) IS NULL), sum_sales - avg_monthly_sales, ((s_store_name) IS NULL), s_store_name limit 100;

--= query_48
PRAGMA AnsiImplicitCrossJoin;
select sum (store_sales.ss_quantity) AS c11
 from store_sales, store, customer_demographics, customer_address, date_dim
 where s_store_sk = ss_store_sk
 and  ss_sold_date_sk = d_date_sk and d_year = 2000
 and
 (
  (
   cd_demo_sk = ss_cdemo_sk
   and
   cd_marital_status = 'M'
   and
   cd_education_status = '4 yr Degree'
   and
   ss_sales_price between 100.00 and 150.00
   )
 or
  (
  cd_demo_sk = ss_cdemo_sk
   and
   cd_marital_status = 'D'
   and
   cd_education_status = '2 yr Degree'
   and
   ss_sales_price between 50.00 and 100.00
  )
 or
 (
  cd_demo_sk = ss_cdemo_sk
  and
   cd_marital_status = 'S'
   and
   cd_education_status = 'College'
   and
   ss_sales_price between 150.00 and 200.00
 )
 )
 and
 (
  (
  ss_addr_sk = ca_address_sk
  and
  ca_country = 'United States'
  and
  ca_state in ('CO', 'OH', 'TX')
  and ss_net_profit between 0 and 2000
  )
 or
  (ss_addr_sk = ca_address_sk
  and
  ca_country = 'United States'
  and
  ca_state in ('OR', 'MN', 'KY')
  and ss_net_profit between 150 and 3000
  )
 or
  (ss_addr_sk = ca_address_sk
  and
  ca_country = 'United States'
  and
  ca_state in ('VA', 'CA', 'MS')
  and ss_net_profit between 50 and 25000
  )
 );

--= query_49
PRAGMA AnsiImplicitCrossJoin;
select  channel, item, return_ratio, return_rank, currency_rank from
 (select
 'web' as channel
 ,web.item AS item
 ,web.return_ratio AS return_ratio
 ,web.return_rank AS return_rank
 ,web.currency_rank AS currency_rank
 from (
    select
     item
    ,return_ratio
    ,currency_ratio
    ,rank() over (order by ((return_ratio) IS NULL), return_ratio ) as return_rank
    ,rank() over (order by ((currency_ratio) IS NULL), currency_ratio ) as currency_rank
    from
    (   select ws.ws_item_sk AS item,
  (cast(sum(coalesce(wr.wr_return_quantity,0)) as Double)/
        cast(sum(coalesce(ws.ws_quantity,0)) as Double )) AS return_ratio,
  (cast(sum(coalesce(wr.wr_return_amt,0)) as Double)/
        cast(sum(coalesce(ws.ws_net_paid,0)) as Double )) AS currency_ratio
 from
         web_sales ws left outer join web_returns wr
            on (ws.ws_order_number = wr.wr_order_number and
            ws.ws_item_sk = wr.wr_item_sk)
                 ,date_dim
        where
            wr.wr_return_amt > 10000
            and ws.ws_net_profit > 1
                         and ws.ws_net_paid > 0
                         and ws.ws_quantity > 0
                         and ws_sold_date_sk = d_date_sk
                         and d_year = 2001
                         and d_moy = 12
        group by ws.ws_item_sk
 ) in_web
 ) web
 where
 (
 web.return_rank <= 10
 or
 web.currency_rank <= 10
 )
 union
 select
 'catalog' as channel
 ,catalog.item AS item
 ,catalog.return_ratio AS return_ratio
 ,catalog.return_rank AS return_rank
 ,catalog.currency_rank AS currency_rank
 from (
    select
     item
    ,return_ratio
    ,currency_ratio
    ,rank() over (order by ((return_ratio) IS NULL), return_ratio ) as return_rank
    ,rank() over (order by ((currency_ratio) IS NULL), currency_ratio ) as currency_rank
    from
    (   select cs.cs_item_sk AS item,
  (cast(sum(coalesce(cr.cr_return_quantity,0)) as Double)/
        cast(sum(coalesce(cs.cs_quantity,0)) as Double )) AS return_ratio,
  (cast(sum(coalesce(cr.cr_return_amount,0)) as Double)/
        cast(sum(coalesce(cs.cs_net_paid,0)) as Double )) AS currency_ratio
 from
        catalog_sales cs left outer join catalog_returns cr
            on (cs.cs_order_number = cr.cr_order_number and
            cs.cs_item_sk = cr.cr_item_sk)
                ,date_dim
        where
            cr.cr_return_amount > 10000
            and cs.cs_net_profit > 1
                         and cs.cs_net_paid > 0
                         and cs.cs_quantity > 0
                         and cs_sold_date_sk = d_date_sk
                         and d_year = 2001
                         and d_moy = 12
                 group by cs.cs_item_sk
 ) in_cat
 ) catalog
 where
 (
 catalog.return_rank <= 10
 or
 catalog.currency_rank <=10
 )
 union
 select
 'store' as channel
 ,store.item AS item
 ,store.return_ratio AS return_ratio
 ,store.return_rank AS return_rank
 ,store.currency_rank AS currency_rank
 from (
    select
     item
    ,return_ratio
    ,currency_ratio
    ,rank() over (order by ((return_ratio) IS NULL), return_ratio ) as return_rank
    ,rank() over (order by ((currency_ratio) IS NULL), currency_ratio ) as currency_rank
    from
    (   select sts.ss_item_sk AS item,
  (cast(sum(coalesce(sr.sr_return_quantity,0)) as Double)/cast(sum(coalesce(sts.ss_quantity,0)) as Double )) AS return_ratio,
  (cast(sum(coalesce(sr.sr_return_amt,0)) as Double)/cast(sum(coalesce(sts.ss_net_paid,0)) as Double )) AS currency_ratio
 from
        store_sales sts left outer join store_returns sr
            on (sts.ss_ticket_number = sr.sr_ticket_number and sts.ss_item_sk = sr.sr_item_sk)
                ,date_dim
        where
            sr.sr_return_amt > 10000
            and sts.ss_net_profit > 1
                         and sts.ss_net_paid > 0
                         and sts.ss_quantity > 0
                         and ss_sold_date_sk = d_date_sk
                         and d_year = 2001
                         and d_moy = 12
        group by sts.ss_item_sk
 ) in_store
 ) store
 where  (
 store.return_rank <= 10
 or
 store.currency_rank <= 10
 )
 ) AS _sq1
 order by ((channel) IS NULL), channel, ((return_rank) IS NULL), return_rank, ((currency_rank) IS NULL), currency_rank, ((item) IS NULL), item limit 100;

--= query_50
PRAGMA AnsiImplicitCrossJoin;
select store.s_store_name AS s_store_name,
  store.s_company_id AS s_company_id,
  store.s_street_number AS s_street_number,
  store.s_street_name AS s_street_name,
  store.s_street_type AS s_street_type,
  store.s_suite_number AS s_suite_number,
  store.s_city AS s_city,
  store.s_county AS s_county,
  store.s_state AS s_state,
  store.s_zip AS s_zip,
  sum(case when (store_returns.sr_returned_date_sk - store_sales.ss_sold_date_sk <= 30 ) then 1 else 0 end) AS `30 days`,
  sum(case when (store_returns.sr_returned_date_sk - store_sales.ss_sold_date_sk > 30) and
                 (store_returns.sr_returned_date_sk - store_sales.ss_sold_date_sk <= 60) then 1 else 0 end ) AS `31-60 days`,
  sum(case when (store_returns.sr_returned_date_sk - store_sales.ss_sold_date_sk > 60) and
                 (store_returns.sr_returned_date_sk - store_sales.ss_sold_date_sk <= 90) then 1 else 0 end) AS `61-90 days`,
  sum(case when (store_returns.sr_returned_date_sk - store_sales.ss_sold_date_sk > 90) and
                 (store_returns.sr_returned_date_sk - store_sales.ss_sold_date_sk <= 120) then 1 else 0 end) AS `91-120 days`,
  sum(case when (store_returns.sr_returned_date_sk - store_sales.ss_sold_date_sk  > 120) then 1 else 0 end) AS `>120 days`
 from
   store_sales
  ,store_returns
  ,store
  ,date_dim d1
  ,date_dim d2
where
    d2.d_year = 2001
and d2.d_moy  = 8
and ss_ticket_number = sr_ticket_number
and ss_item_sk = sr_item_sk
and ss_sold_date_sk   = d1.d_date_sk
and sr_returned_date_sk   = d2.d_date_sk
and ss_customer_sk = sr_customer_sk
and ss_store_sk = s_store_sk
group by store.s_store_name, store.s_company_id, store.s_street_number, store.s_street_name, store.s_street_type, store.s_suite_number, store.s_city, store.s_county, store.s_state, store.s_zip
 order by ((s_store_name) IS NULL), s_store_name, ((s_company_id) IS NULL), s_company_id, ((s_street_number) IS NULL), s_street_number, ((s_street_name) IS NULL), s_street_name, ((s_street_type) IS NULL), s_street_type, ((s_suite_number) IS NULL), s_suite_number, ((s_city) IS NULL), s_city, ((s_county) IS NULL), s_county, ((s_state) IS NULL), s_state, ((s_zip) IS NULL), s_zip limit 100;

--= query_51
PRAGMA AnsiImplicitCrossJoin;
$web_v1 = (select web_sales.ws_item_sk AS item_sk,
  date_dim.d_date AS d_date,
  sum(sum(web_sales.ws_sales_price))
      over (partition by web_sales.ws_item_sk order by ((date_dim.d_date) IS NULL), date_dim.d_date rows between unbounded preceding and current row ) AS cume_sales
 from web_sales
    ,date_dim
where ws_sold_date_sk=d_date_sk
  and d_month_seq between 1200 and 1200+11
  and ws_item_sk is not NULL
group by web_sales.ws_item_sk, date_dim.d_date
 );
$store_v1 = (select store_sales.ss_item_sk AS item_sk,
  date_dim.d_date AS d_date,
  sum(sum(store_sales.ss_sales_price))
      over (partition by store_sales.ss_item_sk order by ((date_dim.d_date) IS NULL), date_dim.d_date rows between unbounded preceding and current row ) AS cume_sales
 from store_sales
    ,date_dim
where ss_sold_date_sk=d_date_sk
  and d_month_seq between 1200 and 1200+11
  and ss_item_sk is not NULL
group by store_sales.ss_item_sk, date_dim.d_date
 );

 select item_sk, d_date || ' 00:00:00 +0000 UTC' AS d_date, web_sales, store_sales, web_cumulative, store_cumulative from (select item_sk
     ,d_date
     ,web_sales
     ,store_sales
     ,max(web_sales)
         over (partition by item_sk order by ((d_date) IS NULL), d_date rows between unbounded preceding and current row ) web_cumulative
     ,max(store_sales)
         over (partition by item_sk order by ((d_date) IS NULL), d_date rows between unbounded preceding and current row ) store_cumulative
     from (select case when web.item_sk is not null then web.item_sk else store.item_sk end item_sk
                 ,case when web.d_date is not null then web.d_date else store.d_date end d_date
                 ,web.cume_sales web_sales
                 ,store.cume_sales store_sales
           from $web_v1 AS web full outer join $store_v1 AS store on (web.item_sk = store.item_sk
                                                          and web.d_date = store.d_date)
          )x )y
where web_cumulative > store_cumulative
order by ((item_sk) IS NULL), item_sk, ((d_date) IS NULL), d_date limit 100;

--= query_52
PRAGMA AnsiImplicitCrossJoin;
select dt.d_year AS d_year,
  item.i_brand_id AS brand_id,
  item.i_brand AS brand,
  sum(store_sales.ss_ext_sales_price) AS ext_price
 from date_dim dt
     ,store_sales
     ,item
 where dt.d_date_sk = store_sales.ss_sold_date_sk
    and store_sales.ss_item_sk = item.i_item_sk
    and item.i_manager_id = 1
    and dt.d_moy=11
    and dt.d_year=2000
 group by dt.d_year, item.i_brand, item.i_brand_id
 order by ((d_year) IS NULL), d_year, ((ext_price) IS NOT NULL), ext_price DESC, ((brand_id) IS NULL), brand_id limit 100;

--= query_53
PRAGMA AnsiImplicitCrossJoin;
select i_manufact_id, sum_sales, avg_quarterly_sales from
(select item.i_manufact_id AS i_manufact_id,
  sum(store_sales.ss_sales_price) AS sum_sales,
  avg(sum(store_sales.ss_sales_price)) over (partition by item.i_manufact_id) AS avg_quarterly_sales
 from item, store_sales, date_dim, store
where ss_item_sk = i_item_sk and
ss_sold_date_sk = d_date_sk and
ss_store_sk = s_store_sk and
d_month_seq in (1200,1200+1,1200+2,1200+3,1200+4,1200+5,1200+6,1200+7,1200+8,1200+9,1200+10,1200+11) and
((i_category in ('Books','Children','Electronics') and
i_class in ('personal','portable','reference','self-help') and
i_brand in ('scholaramalgamalg #14','scholaramalgamalg #7',
        'exportiunivamalg #9','scholaramalgamalg #9'))
or(i_category in ('Women','Music','Men') and
i_class in ('accessories','classical','fragrances','pants') and
i_brand in ('amalgimporto #1','edu packscholar #1','exportiimporto #1',
        'importoamalg #1')))
group by item.i_manufact_id, date_dim.d_qoy
 ) tmp1
where case when avg_quarterly_sales > 0
    then abs (sum_sales - avg_quarterly_sales)/ avg_quarterly_sales
    else null end > 0.1
order by ((avg_quarterly_sales) IS NULL), avg_quarterly_sales, ((sum_sales) IS NULL), sum_sales, ((i_manufact_id) IS NULL), i_manufact_id limit 100;

--= query_54
PRAGMA AnsiImplicitCrossJoin;
$my_customers = (select distinct c_customer_sk
        , c_current_addr_sk
 from
        ( select cs_sold_date_sk sold_date_sk,
                 cs_bill_customer_sk customer_sk,
                 cs_item_sk item_sk
          from   catalog_sales
          union all
          select ws_sold_date_sk sold_date_sk,
                 ws_bill_customer_sk customer_sk,
                 ws_item_sk item_sk
          from   web_sales
         ) cs_or_ws_sales,
         item,
         date_dim,
         customer
 where   sold_date_sk = d_date_sk
         and item_sk = i_item_sk
         and i_category = 'Women'
         and i_class = 'maternity'
         and c_customer_sk = cs_or_ws_sales.customer_sk
         and d_moy = 12
         and d_year = 1998);
$my_revenue = (select my_customers.c_customer_sk AS c_customer_sk,
  sum(store_sales.ss_ext_sales_price) AS revenue
 from   $my_customers AS my_customers,
        store_sales,
        customer_address,
        store,
        date_dim
 where  c_current_addr_sk = ca_address_sk
        and ca_county = s_county
        and ca_state = s_state
        and ss_sold_date_sk = d_date_sk
        and c_customer_sk = ss_customer_sk
        and d_month_seq between (select distinct d_month_seq+1
                                 from   date_dim where d_year = 1998 and d_moy = 12)
                           and  (select distinct d_month_seq+3
                                 from   date_dim where d_year = 1998 and d_moy = 12)
 group by my_customers.c_customer_sk
 );
$segments = (select cast((revenue/50) as Int32) as segment
  from   $my_revenue AS my_revenue);

  select segment AS segment,
  count(*) AS num_customers,
  segment*50 AS segment_base
 from $segments AS segments
 group by segment
 order by ((segment) IS NULL), segment, ((num_customers) IS NULL), num_customers limit 100;

--= query_55
PRAGMA AnsiImplicitCrossJoin;
select item.i_brand_id AS brand_id,
  item.i_brand AS brand,
  sum(store_sales.ss_ext_sales_price) AS ext_price
 from date_dim, store_sales, item
 where d_date_sk = ss_sold_date_sk
    and ss_item_sk = i_item_sk
    and i_manager_id=28
    and d_moy=11
    and d_year=1999
 group by item.i_brand, item.i_brand_id
 order by ((ext_price) IS NOT NULL), ext_price DESC, ((brand_id) IS NULL), brand_id limit 100;

--= query_56
PRAGMA AnsiImplicitCrossJoin;
$ss = (select item.i_item_id AS i_item_id,
  sum(store_sales.ss_ext_sales_price) AS total_sales
 from
    store_sales,
    date_dim,
         customer_address,
         item
 where i_item_id in (select
     i_item_id
from item
where i_color in ('slate','blanched','burnished'))
 and     ss_item_sk              = i_item_sk
 and     ss_sold_date_sk         = d_date_sk
 and     d_year                  = 2001
 and     d_moy                   = 2
 and     ss_addr_sk              = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_item_id
 );
$cs = (select item.i_item_id AS i_item_id,
  sum(catalog_sales.cs_ext_sales_price) AS total_sales
 from
    catalog_sales,
    date_dim,
         customer_address,
         item
 where
         i_item_id               in (select
  i_item_id
from item
where i_color in ('slate','blanched','burnished'))
 and     cs_item_sk              = i_item_sk
 and     cs_sold_date_sk         = d_date_sk
 and     d_year                  = 2001
 and     d_moy                   = 2
 and     cs_bill_addr_sk         = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_item_id
 );
$ws = (select item.i_item_id AS i_item_id,
  sum(web_sales.ws_ext_sales_price) AS total_sales
 from
    web_sales,
    date_dim,
         customer_address,
         item
 where
         i_item_id               in (select
  i_item_id
from item
where i_color in ('slate','blanched','burnished'))
 and     ws_item_sk              = i_item_sk
 and     ws_sold_date_sk         = d_date_sk
 and     d_year                  = 2001
 and     d_moy                   = 2
 and     ws_bill_addr_sk         = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_item_id
 );

  select i_item_id AS i_item_id,
  sum(total_sales) AS total_sales
 from  (select * from $ss AS ss
        union all
        select * from $cs AS cs
        union all
        select * from $ws AS ws) tmp1
 group by i_item_id
 order by ((total_sales) IS NULL), total_sales, ((i_item_id) IS NULL), i_item_id limit 100;

--= query_57
PRAGMA AnsiImplicitCrossJoin;
$v1 = (select item.i_category AS i_category,
  item.i_brand AS i_brand,
  call_center.cc_name AS cc_name,
  date_dim.d_year AS d_year,
  date_dim.d_moy AS d_moy,
  sum(catalog_sales.cs_sales_price) AS sum_sales,
  avg(sum(catalog_sales.cs_sales_price)) over
          (partition by item.i_category, item.i_brand,
                     call_center.cc_name, date_dim.d_year) AS avg_monthly_sales,
  rank() over
          (partition by item.i_category, item.i_brand,
                     call_center.cc_name
           order by ((date_dim.d_year) IS NULL), date_dim.d_year, ((date_dim.d_moy) IS NULL), date_dim.d_moy ) AS rn
 from item, catalog_sales, date_dim, call_center
 where cs_item_sk = i_item_sk and
       cs_sold_date_sk = d_date_sk and
       cc_call_center_sk= cs_call_center_sk and
       (
         d_year = 1999 or
         ( d_year = 1999-1 and d_moy =12) or
         ( d_year = 1999+1 and d_moy =1)
       )
 group by item.i_category, item.i_brand, call_center.cc_name, date_dim.d_year, date_dim.d_moy
 );
$v2 = (select v1.i_category AS i_category, v1.i_brand AS i_brand, v1.cc_name AS cc_name
        ,v1.d_year AS d_year
        ,v1.avg_monthly_sales AS avg_monthly_sales
        ,v1.sum_sales AS sum_sales, v1_lag.sum_sales psum, v1_lead.sum_sales nsum
 from $v1 AS v1, $v1 AS v1_lag, $v1 AS v1_lead
 where v1.i_category = v1_lag.i_category and
       v1.i_category = v1_lead.i_category and
       v1.i_brand = v1_lag.i_brand and
       v1.i_brand = v1_lead.i_brand and
       v1. cc_name = v1_lag. cc_name and
       v1. cc_name = v1_lead. cc_name and
       v1.rn = v1_lag.rn + 1 and
       v1.rn = v1_lead.rn - 1);

  select i_category, i_brand, cc_name, d_year, avg_monthly_sales, sum_sales, psum, nsum from $v2 AS v2
 where  d_year = 1999 and
        avg_monthly_sales > 0 and
        case when avg_monthly_sales > 0 then abs(sum_sales - avg_monthly_sales) / avg_monthly_sales else null end > 0.1
 order by ((sum_sales - avg_monthly_sales) IS NULL), sum_sales - avg_monthly_sales, ((cc_name) IS NULL), cc_name limit 100;

--= query_58
PRAGMA AnsiImplicitCrossJoin;
$ss_items = (select item.i_item_id AS item_id,
  sum(store_sales.ss_ext_sales_price) AS ss_item_rev
 from store_sales
     ,item
     ,date_dim
 where ss_item_sk = i_item_sk
   and d_date in (select d_date
                  from date_dim
                  where d_week_seq = (select d_week_seq
                                      from date_dim
                                      where d_date = '2000-01-03'))
   and ss_sold_date_sk   = d_date_sk
 group by item.i_item_id
 );
$cs_items = (select item.i_item_id AS item_id,
  sum(catalog_sales.cs_ext_sales_price) AS cs_item_rev
 from catalog_sales
      ,item
      ,date_dim
 where cs_item_sk = i_item_sk
  and  d_date in (select d_date
                  from date_dim
                  where d_week_seq = (select d_week_seq
                                      from date_dim
                                      where d_date = '2000-01-03'))
  and  cs_sold_date_sk = d_date_sk
 group by item.i_item_id
 );
$ws_items = (select item.i_item_id AS item_id,
  sum(web_sales.ws_ext_sales_price) AS ws_item_rev
 from web_sales
      ,item
      ,date_dim
 where ws_item_sk = i_item_sk
  and  d_date in (select d_date
                  from date_dim
                  where d_week_seq =(select d_week_seq
                                     from date_dim
                                     where d_date = '2000-01-03'))
  and ws_sold_date_sk   = d_date_sk
 group by item.i_item_id
 );

  select  ss_items.item_id AS item_id
       ,ss_item_rev
       ,ss_item_rev/((ss_item_rev+cs_item_rev+ws_item_rev)/3) * 100 ss_dev
       ,cs_item_rev
       ,cs_item_rev/((ss_item_rev+cs_item_rev+ws_item_rev)/3) * 100 cs_dev
       ,ws_item_rev
       ,ws_item_rev/((ss_item_rev+cs_item_rev+ws_item_rev)/3) * 100 ws_dev
       ,(ss_item_rev+cs_item_rev+ws_item_rev)/3 average
 from $ss_items AS ss_items,$cs_items AS cs_items,$ws_items AS ws_items
 where ss_items.item_id=cs_items.item_id
   and ss_items.item_id=ws_items.item_id
   and ss_item_rev between 0.9 * cs_item_rev and 1.1 * cs_item_rev
   and ss_item_rev between 0.9 * ws_item_rev and 1.1 * ws_item_rev
   and cs_item_rev between 0.9 * ss_item_rev and 1.1 * ss_item_rev
   and cs_item_rev between 0.9 * ws_item_rev and 1.1 * ws_item_rev
   and ws_item_rev between 0.9 * ss_item_rev and 1.1 * ss_item_rev
   and ws_item_rev between 0.9 * cs_item_rev and 1.1 * cs_item_rev
 order by ((item_id) IS NULL), item_id, ((ss_item_rev) IS NULL), ss_item_rev limit 100;

--= query_59
PRAGMA AnsiImplicitCrossJoin;
$wss = (select date_dim.d_week_seq AS d_week_seq,
  store_sales.ss_store_sk AS ss_store_sk,
  sum(case when (date_dim.d_day_name='Sunday') then store_sales.ss_sales_price else null end) AS sun_sales,
  sum(case when (date_dim.d_day_name='Monday') then store_sales.ss_sales_price else null end) AS mon_sales,
  sum(case when (date_dim.d_day_name='Tuesday') then store_sales.ss_sales_price else  null end) AS tue_sales,
  sum(case when (date_dim.d_day_name='Wednesday') then store_sales.ss_sales_price else null end) AS wed_sales,
  sum(case when (date_dim.d_day_name='Thursday') then store_sales.ss_sales_price else null end) AS thu_sales,
  sum(case when (date_dim.d_day_name='Friday') then store_sales.ss_sales_price else null end) AS fri_sales,
  sum(case when (date_dim.d_day_name='Saturday') then store_sales.ss_sales_price else null end) AS sat_sales
 from store_sales,date_dim
 where d_date_sk = ss_sold_date_sk
 group by date_dim.d_week_seq, store_sales.ss_store_sk
 );

  select  s_store_name1,s_store_id1,d_week_seq1
       ,sun_sales1/sun_sales2,mon_sales1/mon_sales2
       ,tue_sales1/tue_sales2,wed_sales1/wed_sales2,thu_sales1/thu_sales2
       ,fri_sales1/fri_sales2,sat_sales1/sat_sales2
 from
 (select s_store_name s_store_name1,wss.d_week_seq d_week_seq1
        ,s_store_id s_store_id1,sun_sales sun_sales1
        ,mon_sales mon_sales1,tue_sales tue_sales1
        ,wed_sales wed_sales1,thu_sales thu_sales1
        ,fri_sales fri_sales1,sat_sales sat_sales1
  from $wss AS wss,store,date_dim d
  where d.d_week_seq = wss.d_week_seq and
        ss_store_sk = s_store_sk and
        d_month_seq between 1212 and 1212 + 11) y,
 (select s_store_name s_store_name2,wss.d_week_seq d_week_seq2
        ,s_store_id s_store_id2,sun_sales sun_sales2
        ,mon_sales mon_sales2,tue_sales tue_sales2
        ,wed_sales wed_sales2,thu_sales thu_sales2
        ,fri_sales fri_sales2,sat_sales sat_sales2
  from $wss AS wss,store,date_dim d
  where d.d_week_seq = wss.d_week_seq and
        ss_store_sk = s_store_sk and
        d_month_seq between 1212+ 12 and 1212 + 23) x
 where s_store_id1=s_store_id2
   and d_week_seq1=d_week_seq2-52
 order by ((s_store_name1) IS NULL), s_store_name1, ((s_store_id1) IS NULL), s_store_id1, ((d_week_seq1) IS NULL), d_week_seq1 limit 100;

--= query_60
PRAGMA AnsiImplicitCrossJoin;
$ss = (select item.i_item_id AS i_item_id,
  sum(store_sales.ss_ext_sales_price) AS total_sales
 from
    store_sales,
    date_dim,
         customer_address,
         item
 where
         i_item_id in (select
  i_item_id
from
 item
where i_category in ('Music'))
 and     ss_item_sk              = i_item_sk
 and     ss_sold_date_sk         = d_date_sk
 and     d_year                  = 1998
 and     d_moy                   = 9
 and     ss_addr_sk              = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_item_id
 );
$cs = (select item.i_item_id AS i_item_id,
  sum(catalog_sales.cs_ext_sales_price) AS total_sales
 from
    catalog_sales,
    date_dim,
         customer_address,
         item
 where
         i_item_id               in (select
  i_item_id
from
 item
where i_category in ('Music'))
 and     cs_item_sk              = i_item_sk
 and     cs_sold_date_sk         = d_date_sk
 and     d_year                  = 1998
 and     d_moy                   = 9
 and     cs_bill_addr_sk         = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_item_id
 );
$ws = (select item.i_item_id AS i_item_id,
  sum(web_sales.ws_ext_sales_price) AS total_sales
 from
    web_sales,
    date_dim,
         customer_address,
         item
 where
         i_item_id               in (select
  i_item_id
from
 item
where i_category in ('Music'))
 and     ws_item_sk              = i_item_sk
 and     ws_sold_date_sk         = d_date_sk
 and     d_year                  = 1998
 and     d_moy                   = 9
 and     ws_bill_addr_sk         = ca_address_sk
 and     ca_gmt_offset           = -5
 group by item.i_item_id
 );

  select i_item_id AS i_item_id,
  sum(total_sales) AS total_sales
 from  (select * from $ss AS ss
        union all
        select * from $cs AS cs
        union all
        select * from $ws AS ws) tmp1
 group by i_item_id
 order by ((i_item_id) IS NULL), i_item_id, ((total_sales) IS NULL), total_sales limit 100;

--= query_61
PRAGMA AnsiImplicitCrossJoin;
select  promotions,total,cast(promotions as Double)/cast(total as Double)*100
from
  (select sum(store_sales.ss_ext_sales_price) AS promotions
 from  store_sales
        ,store
        ,promotion
        ,date_dim
        ,customer
        ,customer_address
        ,item
   where ss_sold_date_sk = d_date_sk
   and   ss_store_sk = s_store_sk
   and   ss_promo_sk = p_promo_sk
   and   ss_customer_sk= c_customer_sk
   and   ca_address_sk = c_current_addr_sk
   and   ss_item_sk = i_item_sk
   and   ca_gmt_offset = -5
   and   i_category = 'Jewelry'
   and   (p_channel_dmail = 'Y' or p_channel_email = 'Y' or p_channel_tv = 'Y')
   and   s_gmt_offset = -5
   and   d_year = 1998
   and   d_moy  = 11) promotional_sales,
  (select sum(store_sales.ss_ext_sales_price) AS total
 from  store_sales
        ,store
        ,date_dim
        ,customer
        ,customer_address
        ,item
   where ss_sold_date_sk = d_date_sk
   and   ss_store_sk = s_store_sk
   and   ss_customer_sk= c_customer_sk
   and   ca_address_sk = c_current_addr_sk
   and   ss_item_sk = i_item_sk
   and   ca_gmt_offset = -5
   and   i_category = 'Jewelry'
   and   s_gmt_offset = -5
   and   d_year = 1998
   and   d_moy  = 11) all_sales
order by ((promotions) IS NULL), promotions, ((total) IS NULL), total limit 100;

--= query_62
PRAGMA AnsiImplicitCrossJoin;
select gk12,
  ship_mode.sm_type AS sm_type,
  web_site.web_name AS web_name,
  sum(case when (web_sales.ws_ship_date_sk - web_sales.ws_sold_date_sk <= 30 ) then 1 else 0 end) AS `30 days`,
  sum(case when (web_sales.ws_ship_date_sk - web_sales.ws_sold_date_sk > 30) and
                 (web_sales.ws_ship_date_sk - web_sales.ws_sold_date_sk <= 60) then 1 else 0 end ) AS `31-60 days`,
  sum(case when (web_sales.ws_ship_date_sk - web_sales.ws_sold_date_sk > 60) and
                 (web_sales.ws_ship_date_sk - web_sales.ws_sold_date_sk <= 90) then 1 else 0 end) AS `61-90 days`,
  sum(case when (web_sales.ws_ship_date_sk - web_sales.ws_sold_date_sk > 90) and
                 (web_sales.ws_ship_date_sk - web_sales.ws_sold_date_sk <= 120) then 1 else 0 end) AS `91-120 days`,
  sum(case when (web_sales.ws_ship_date_sk - web_sales.ws_sold_date_sk  > 120) then 1 else 0 end) AS `>120 days`
 from
   web_sales
  ,warehouse
  ,ship_mode
  ,web_site
  ,date_dim
where
    d_month_seq between 1200 and 1200 + 11
and ws_ship_date_sk   = d_date_sk
and ws_warehouse_sk   = w_warehouse_sk
and ws_ship_mode_sk   = sm_ship_mode_sk
and ws_web_site_sk    = web_site_sk
group by Unicode::Substring(warehouse.w_warehouse_name, CAST(0 AS Uint32), CAST(20 AS Uint32)) AS gk12, ship_mode.sm_type, web_site.web_name
 order by ((gk12) IS NULL), gk12, ((sm_type) IS NULL), sm_type, ((web_name) IS NULL), web_name limit 100;

--= query_63
PRAGMA AnsiImplicitCrossJoin;
select i_manager_id, sum_sales, avg_monthly_sales from (select item.i_manager_id AS i_manager_id,
  sum(store_sales.ss_sales_price) AS sum_sales,
  avg(sum(store_sales.ss_sales_price)) over (partition by item.i_manager_id) AS avg_monthly_sales
 from item
          ,store_sales
          ,date_dim
          ,store
      where ss_item_sk = i_item_sk
        and ss_sold_date_sk = d_date_sk
        and ss_store_sk = s_store_sk
        and d_month_seq in (1200,1200+1,1200+2,1200+3,1200+4,1200+5,1200+6,1200+7,1200+8,1200+9,1200+10,1200+11)
        and ((    i_category in ('Books','Children','Electronics')
              and i_class in ('personal','portable','reference','self-help')
              and i_brand in ('scholaramalgamalg #14','scholaramalgamalg #7',
                          'exportiunivamalg #9','scholaramalgamalg #9'))
           or(    i_category in ('Women','Music','Men')
              and i_class in ('accessories','classical','fragrances','pants')
              and i_brand in ('amalgimporto #1','edu packscholar #1','exportiimporto #1',
                         'importoamalg #1')))
group by item.i_manager_id, date_dim.d_moy
 ) tmp1
where case when avg_monthly_sales > 0 then abs (sum_sales - avg_monthly_sales) / avg_monthly_sales else null end > 0.1
order by ((i_manager_id) IS NULL), i_manager_id, ((avg_monthly_sales) IS NULL), avg_monthly_sales, ((sum_sales) IS NULL), sum_sales limit 100;

--= query_64
PRAGMA AnsiImplicitCrossJoin;
$cs_ui = (select catalog_sales.cs_item_sk AS cs_item_sk,
  sum(catalog_sales.cs_ext_list_price) AS sale,
  sum(catalog_returns.cr_refunded_cash+catalog_returns.cr_reversed_charge+catalog_returns.cr_store_credit) AS refund
 from catalog_sales
      ,catalog_returns
  where cs_item_sk = cr_item_sk
    and cs_order_number = cr_order_number
  group by catalog_sales.cs_item_sk
 having sum(catalog_sales.cs_ext_list_price)>2*sum(catalog_returns.cr_refunded_cash+catalog_returns.cr_reversed_charge+catalog_returns.cr_store_credit));
$cross_sales = (select item.i_product_name AS product_name,
  item.i_item_sk AS item_sk,
  store.s_store_name AS store_name,
  store.s_zip AS store_zip,
  ad1.ca_street_number AS b_street_number,
  ad1.ca_street_name AS b_street_name,
  ad1.ca_city AS b_city,
  ad1.ca_zip AS b_zip,
  ad2.ca_street_number AS c_street_number,
  ad2.ca_street_name AS c_street_name,
  ad2.ca_city AS c_city,
  ad2.ca_zip AS c_zip,
  d1.d_year AS syear,
  d2.d_year AS fsyear,
  d3.d_year AS s2year,
  count(*) AS cnt,
  sum(store_sales.ss_wholesale_cost) AS s1,
  sum(store_sales.ss_list_price) AS s2,
  sum(store_sales.ss_coupon_amt) AS s3
 FROM   store_sales
        ,store_returns
        ,$cs_ui AS cs_ui
        ,date_dim d1
        ,date_dim d2
        ,date_dim d3
        ,store
        ,customer
        ,customer_demographics cd1
        ,customer_demographics cd2
        ,promotion
        ,household_demographics hd1
        ,household_demographics hd2
        ,customer_address ad1
        ,customer_address ad2
        ,income_band ib1
        ,income_band ib2
        ,item
  WHERE  ss_store_sk = s_store_sk AND
         ss_sold_date_sk = d1.d_date_sk AND
         ss_customer_sk = c_customer_sk AND
         ss_cdemo_sk= cd1.cd_demo_sk AND
         ss_hdemo_sk = hd1.hd_demo_sk AND
         ss_addr_sk = ad1.ca_address_sk and
         ss_item_sk = i_item_sk and
         ss_item_sk = sr_item_sk and
         ss_ticket_number = sr_ticket_number and
         ss_item_sk = cs_ui.cs_item_sk and
         c_current_cdemo_sk = cd2.cd_demo_sk AND
         c_current_hdemo_sk = hd2.hd_demo_sk AND
         c_current_addr_sk = ad2.ca_address_sk and
         c_first_sales_date_sk = d2.d_date_sk and
         c_first_shipto_date_sk = d3.d_date_sk and
         ss_promo_sk = p_promo_sk and
         hd1.hd_income_band_sk = ib1.ib_income_band_sk and
         hd2.hd_income_band_sk = ib2.ib_income_band_sk and
         cd1.cd_marital_status <> cd2.cd_marital_status and
         i_color in ('purple','burlywood','indian','spring','floral','medium') and
         i_current_price between 64 and 64 + 10 and
         i_current_price between 64 + 1 and 64 + 15
group by item.i_product_name, item.i_item_sk, store.s_store_name, store.s_zip, ad1.ca_street_number, ad1.ca_street_name, ad1.ca_city, ad1.ca_zip, ad2.ca_street_number, ad2.ca_street_name, ad2.ca_city, ad2.ca_zip, d1.d_year, d2.d_year, d3.d_year
 );

select cs1.product_name AS product_name
     ,cs1.store_name AS store_name
     ,cs1.store_zip AS store_zip
     ,cs1.b_street_number AS b_street_number
     ,cs1.b_street_name AS b_street_name
     ,cs1.b_city AS b_city
     ,cs1.b_zip AS b_zip
     ,cs1.c_street_number AS c_street_number
     ,cs1.c_street_name AS c_street_name
     ,cs1.c_city AS c_city
     ,cs1.c_zip AS c_zip
     ,cs1.syear
     ,cs1.cnt
     ,cs1.s1 as s11
     ,cs1.s2 as s21
     ,cs1.s3 as s31
     ,cs2.s1 as s12
     ,cs2.s2 as s22
     ,cs2.s3 as s32
     ,cs2.syear
     ,cs2.cnt
from $cross_sales AS cs1,$cross_sales AS cs2
where cs1.item_sk=cs2.item_sk and
     cs1.syear = 1999 and
     cs2.syear = 1999 + 1 and
     cs2.cnt <= cs1.cnt and
     cs1.store_name = cs2.store_name and
     cs1.store_zip = cs2.store_zip
order by ((cs1.product_name) IS NULL), cs1.product_name, ((cs1.store_name) IS NULL), cs1.store_name, ((cs2.cnt) IS NULL), cs2.cnt, ((cs1.s1) IS NULL), cs1.s1, ((cs2.s1) IS NULL), cs2.s1;

--= query_65
PRAGMA AnsiImplicitCrossJoin;
select
    s_store_name,
    i_item_desc,
    sc.revenue AS revenue,
    i_current_price,
    i_wholesale_cost,
    i_brand
 from store, item,
     (select ss_store_sk AS ss_store_sk,
  avg(revenue) AS ave
 from
        (select store_sales.ss_store_sk AS ss_store_sk,
  store_sales.ss_item_sk AS ss_item_sk,
  sum(store_sales.ss_sales_price) AS revenue
 from store_sales, date_dim
        where ss_sold_date_sk = d_date_sk and d_month_seq between 1176 and 1176+11
        group by store_sales.ss_store_sk, store_sales.ss_item_sk
 ) sa
    group by ss_store_sk
 ) sb,
     (select store_sales.ss_store_sk AS ss_store_sk,
  store_sales.ss_item_sk AS ss_item_sk,
  sum(store_sales.ss_sales_price) AS revenue
 from store_sales, date_dim
    where ss_sold_date_sk = d_date_sk and d_month_seq between 1176 and 1176+11
    group by store_sales.ss_store_sk, store_sales.ss_item_sk
 ) sc
 where sb.ss_store_sk = sc.ss_store_sk and
       sc.revenue <= 0.1 * sb.ave and
       s_store_sk = sc.ss_store_sk and
       i_item_sk = sc.ss_item_sk
 order by ((s_store_name) IS NULL), s_store_name, ((i_item_desc) IS NULL), i_item_desc limit 100;

--= query_66
PRAGMA AnsiImplicitCrossJoin;
select w_warehouse_name AS w_warehouse_name,
  w_warehouse_sq_ft AS w_warehouse_sq_ft,
  w_city AS w_city,
  w_county AS w_county,
  w_state AS w_state,
  w_country AS w_country,
  ship_carriers AS ship_carriers,
  year AS year,
  sum(jan_sales) AS jan_sales,
  sum(feb_sales) AS feb_sales,
  sum(mar_sales) AS mar_sales,
  sum(apr_sales) AS apr_sales,
  sum(may_sales) AS may_sales,
  sum(jun_sales) AS jun_sales,
  sum(jul_sales) AS jul_sales,
  sum(aug_sales) AS aug_sales,
  sum(sep_sales) AS sep_sales,
  sum(oct_sales) AS oct_sales,
  sum(nov_sales) AS nov_sales,
  sum(dec_sales) AS dec_sales,
  sum(jan_sales/w_warehouse_sq_ft) AS jan_sales_per_sq_foot,
  sum(feb_sales/w_warehouse_sq_ft) AS feb_sales_per_sq_foot,
  sum(mar_sales/w_warehouse_sq_ft) AS mar_sales_per_sq_foot,
  sum(apr_sales/w_warehouse_sq_ft) AS apr_sales_per_sq_foot,
  sum(may_sales/w_warehouse_sq_ft) AS may_sales_per_sq_foot,
  sum(jun_sales/w_warehouse_sq_ft) AS jun_sales_per_sq_foot,
  sum(jul_sales/w_warehouse_sq_ft) AS jul_sales_per_sq_foot,
  sum(aug_sales/w_warehouse_sq_ft) AS aug_sales_per_sq_foot,
  sum(sep_sales/w_warehouse_sq_ft) AS sep_sales_per_sq_foot,
  sum(oct_sales/w_warehouse_sq_ft) AS oct_sales_per_sq_foot,
  sum(nov_sales/w_warehouse_sq_ft) AS nov_sales_per_sq_foot,
  sum(dec_sales/w_warehouse_sq_ft) AS dec_sales_per_sq_foot,
  sum(jan_net) AS jan_net,
  sum(feb_net) AS feb_net,
  sum(mar_net) AS mar_net,
  sum(apr_net) AS apr_net,
  sum(may_net) AS may_net,
  sum(jun_net) AS jun_net,
  sum(jul_net) AS jul_net,
  sum(aug_net) AS aug_net,
  sum(sep_net) AS sep_net,
  sum(oct_net) AS oct_net,
  sum(nov_net) AS nov_net,
  sum(dec_net) AS dec_net
 from (
     select warehouse.w_warehouse_name AS w_warehouse_name,
  warehouse.w_warehouse_sq_ft AS w_warehouse_sq_ft,
  warehouse.w_city AS w_city,
  warehouse.w_county AS w_county,
  warehouse.w_state AS w_state,
  warehouse.w_country AS w_country,
  'DHL' || ',' || 'BARIAN' AS ship_carriers,
  date_dim.d_year AS year,
  sum(case when date_dim.d_moy = 1
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS jan_sales,
  sum(case when date_dim.d_moy = 2
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS feb_sales,
  sum(case when date_dim.d_moy = 3
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS mar_sales,
  sum(case when date_dim.d_moy = 4
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS apr_sales,
  sum(case when date_dim.d_moy = 5
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS may_sales,
  sum(case when date_dim.d_moy = 6
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS jun_sales,
  sum(case when date_dim.d_moy = 7
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS jul_sales,
  sum(case when date_dim.d_moy = 8
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS aug_sales,
  sum(case when date_dim.d_moy = 9
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS sep_sales,
  sum(case when date_dim.d_moy = 10
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS oct_sales,
  sum(case when date_dim.d_moy = 11
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS nov_sales,
  sum(case when date_dim.d_moy = 12
        then web_sales.ws_ext_sales_price* web_sales.ws_quantity else 0 end) AS dec_sales,
  sum(case when date_dim.d_moy = 1
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS jan_net,
  sum(case when date_dim.d_moy = 2
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS feb_net,
  sum(case when date_dim.d_moy = 3
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS mar_net,
  sum(case when date_dim.d_moy = 4
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS apr_net,
  sum(case when date_dim.d_moy = 5
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS may_net,
  sum(case when date_dim.d_moy = 6
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS jun_net,
  sum(case when date_dim.d_moy = 7
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS jul_net,
  sum(case when date_dim.d_moy = 8
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS aug_net,
  sum(case when date_dim.d_moy = 9
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS sep_net,
  sum(case when date_dim.d_moy = 10
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS oct_net,
  sum(case when date_dim.d_moy = 11
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS nov_net,
  sum(case when date_dim.d_moy = 12
        then web_sales.ws_net_paid * web_sales.ws_quantity else 0 end) AS dec_net
 from
          web_sales
         ,warehouse
         ,date_dim
         ,time_dim
      ,ship_mode
     where
            ws_warehouse_sk =  w_warehouse_sk
        and ws_sold_date_sk = d_date_sk
        and ws_sold_time_sk = t_time_sk
    and ws_ship_mode_sk = sm_ship_mode_sk
        and d_year = 2001
    and t_time between 30838 and 30838+28800
    and sm_carrier in ('DHL','BARIAN')
     group by warehouse.w_warehouse_name, warehouse.w_warehouse_sq_ft, warehouse.w_city, warehouse.w_county, warehouse.w_state, warehouse.w_country, date_dim.d_year
 union all
     select warehouse.w_warehouse_name AS w_warehouse_name,
  warehouse.w_warehouse_sq_ft AS w_warehouse_sq_ft,
  warehouse.w_city AS w_city,
  warehouse.w_county AS w_county,
  warehouse.w_state AS w_state,
  warehouse.w_country AS w_country,
  'DHL' || ',' || 'BARIAN' AS ship_carriers,
  date_dim.d_year AS year,
  sum(case when date_dim.d_moy = 1
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS jan_sales,
  sum(case when date_dim.d_moy = 2
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS feb_sales,
  sum(case when date_dim.d_moy = 3
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS mar_sales,
  sum(case when date_dim.d_moy = 4
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS apr_sales,
  sum(case when date_dim.d_moy = 5
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS may_sales,
  sum(case when date_dim.d_moy = 6
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS jun_sales,
  sum(case when date_dim.d_moy = 7
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS jul_sales,
  sum(case when date_dim.d_moy = 8
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS aug_sales,
  sum(case when date_dim.d_moy = 9
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS sep_sales,
  sum(case when date_dim.d_moy = 10
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS oct_sales,
  sum(case when date_dim.d_moy = 11
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS nov_sales,
  sum(case when date_dim.d_moy = 12
        then catalog_sales.cs_sales_price* catalog_sales.cs_quantity else 0 end) AS dec_sales,
  sum(case when date_dim.d_moy = 1
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS jan_net,
  sum(case when date_dim.d_moy = 2
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS feb_net,
  sum(case when date_dim.d_moy = 3
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS mar_net,
  sum(case when date_dim.d_moy = 4
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS apr_net,
  sum(case when date_dim.d_moy = 5
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS may_net,
  sum(case when date_dim.d_moy = 6
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS jun_net,
  sum(case when date_dim.d_moy = 7
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS jul_net,
  sum(case when date_dim.d_moy = 8
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS aug_net,
  sum(case when date_dim.d_moy = 9
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS sep_net,
  sum(case when date_dim.d_moy = 10
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS oct_net,
  sum(case when date_dim.d_moy = 11
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS nov_net,
  sum(case when date_dim.d_moy = 12
        then catalog_sales.cs_net_paid * catalog_sales.cs_quantity else 0 end) AS dec_net
 from
          catalog_sales
         ,warehouse
         ,date_dim
         ,time_dim
     ,ship_mode
     where
            cs_warehouse_sk =  w_warehouse_sk
        and cs_sold_date_sk = d_date_sk
        and cs_sold_time_sk = t_time_sk
    and cs_ship_mode_sk = sm_ship_mode_sk
        and d_year = 2001
    and t_time between 30838 AND 30838+28800
    and sm_carrier in ('DHL','BARIAN')
     group by warehouse.w_warehouse_name, warehouse.w_warehouse_sq_ft, warehouse.w_city, warehouse.w_county, warehouse.w_state, warehouse.w_country, date_dim.d_year
 ) x
 group by w_warehouse_name, w_warehouse_sq_ft, w_city, w_county, w_state, w_country, ship_carriers, year
 order by ((w_warehouse_name) IS NULL), w_warehouse_name limit 100;

--= query_67
PRAGMA AnsiImplicitCrossJoin;
select i_category, i_class, i_brand, i_product_name, d_year, d_qoy, d_moy, s_store_id, sumsales, rk from (select i_category
            ,i_class
            ,i_brand
            ,i_product_name
            ,d_year
            ,d_qoy
            ,d_moy
            ,s_store_id
            ,sumsales
            ,rank() over (partition by i_category order by ((sumsales) IS NOT NULL), sumsales DESC ) rk
      from (select item.i_category AS i_category,
  item.i_class AS i_class,
  item.i_brand AS i_brand,
  item.i_product_name AS i_product_name,
  date_dim.d_year AS d_year,
  date_dim.d_qoy AS d_qoy,
  date_dim.d_moy AS d_moy,
  store.s_store_id AS s_store_id,
  sum(coalesce(store_sales.ss_sales_price*store_sales.ss_quantity,0)) AS sumsales
 from store_sales
                ,date_dim
                ,store
                ,item
       where  ss_sold_date_sk=d_date_sk
          and ss_item_sk=i_item_sk
          and ss_store_sk = s_store_sk
          and d_month_seq between 1200 and 1200+11
       group by   rollup(item.i_category, item.i_class, item.i_brand, item.i_product_name, date_dim.d_year, date_dim.d_qoy, date_dim.d_moy,store.s_store_id)
 )dw1) dw2
where rk <= 100
order by ((i_category) IS NULL), i_category, ((i_class) IS NULL), i_class, ((i_brand) IS NULL), i_brand, ((i_product_name) IS NULL), i_product_name, ((d_year) IS NULL), d_year, ((d_qoy) IS NULL), d_qoy, ((d_moy) IS NULL), d_moy, ((s_store_id) IS NULL), s_store_id, ((sumsales) IS NULL), sumsales, ((rk) IS NULL), rk limit 100;

--= query_68
PRAGMA AnsiImplicitCrossJoin;
select  c_last_name
       ,c_first_name
       ,ca_city
       ,bought_city
       ,ss_ticket_number
       ,extended_price
       ,extended_tax
       ,list_price
 from (select store_sales.ss_ticket_number AS ss_ticket_number,
  store_sales.ss_customer_sk AS ss_customer_sk,
  customer_address.ca_city AS bought_city,
  sum(store_sales.ss_ext_sales_price) AS extended_price,
  sum(store_sales.ss_ext_list_price) AS list_price,
  sum(store_sales.ss_ext_tax) AS extended_tax
 from store_sales
           ,date_dim
           ,store
           ,household_demographics
           ,customer_address
       where store_sales.ss_sold_date_sk = date_dim.d_date_sk
         and store_sales.ss_store_sk = store.s_store_sk
        and store_sales.ss_hdemo_sk = household_demographics.hd_demo_sk
        and store_sales.ss_addr_sk = customer_address.ca_address_sk
        and date_dim.d_dom between 1 and 2
        and (household_demographics.hd_dep_count = 4 or
             household_demographics.hd_vehicle_count= 3)
        and date_dim.d_year in (1999,1999+1,1999+2)
        and store.s_city in ('Fairview','Midway')
       group by store_sales.ss_ticket_number, store_sales.ss_customer_sk, store_sales.ss_addr_sk, customer_address.ca_city
 ) dn
      ,customer
      ,customer_address current_addr
 where ss_customer_sk = c_customer_sk
   and customer.c_current_addr_sk = current_addr.ca_address_sk
   and current_addr.ca_city <> bought_city
 order by ((c_last_name) IS NULL), c_last_name, ((ss_ticket_number) IS NULL), ss_ticket_number limit 100;

--= query_69
PRAGMA AnsiImplicitCrossJoin;
select customer_demographics.cd_gender AS cd_gender,
  customer_demographics.cd_marital_status AS cd_marital_status,
  customer_demographics.cd_education_status AS cd_education_status,
  count(*) AS cnt1,
  customer_demographics.cd_purchase_estimate AS cd_purchase_estimate,
  count(*) AS cnt2,
  customer_demographics.cd_credit_rating AS cd_credit_rating,
  count(*) AS cnt3
 from
  customer c,customer_address ca,customer_demographics
 where
  c.c_current_addr_sk = ca.ca_address_sk and
  ca_state in ('KY','GA','NM') and
  cd_demo_sk = c.c_current_cdemo_sk and
  c.c_customer_sk in (select ss_customer_sk from store_sales,date_dim where ss_sold_date_sk = d_date_sk and
                d_year = 2001 and
                d_moy between 4 and 4+2) and
   (c.c_customer_sk not in (select ws_bill_customer_sk from web_sales,date_dim where ws_sold_date_sk = d_date_sk and
                  d_year = 2001 and
                  d_moy between 4 and 4+2 and
                  ws_bill_customer_sk is not null) and
    c.c_customer_sk not in (select cs_ship_customer_sk from catalog_sales,date_dim where cs_sold_date_sk = d_date_sk and
                  d_year = 2001 and
                  d_moy between 4 and 4+2 and
                  cs_ship_customer_sk is not null))
 group by customer_demographics.cd_gender, customer_demographics.cd_marital_status, customer_demographics.cd_education_status, customer_demographics.cd_purchase_estimate, customer_demographics.cd_credit_rating
 order by ((cd_gender) IS NULL), cd_gender, ((cd_marital_status) IS NULL), cd_marital_status, ((cd_education_status) IS NULL), cd_education_status, ((cd_purchase_estimate) IS NULL), cd_purchase_estimate, ((cd_credit_rating) IS NULL), cd_credit_rating limit 100;

--= query_70
PRAGMA AnsiImplicitCrossJoin;
select sum(store_sales.ss_net_profit) AS total_sum,
  store.s_state AS s_state,
  store.s_county AS s_county,
  grouping(store.s_state)+grouping(store.s_county) AS lochierarchy,
  rank() over (
    partition by grouping(store.s_state)+grouping(store.s_county),
    case when grouping(store.s_county) = 0 then store.s_state  else null end
    order by ((sum(store_sales.ss_net_profit)) IS NOT NULL), sum(store_sales.ss_net_profit) DESC ) AS rank_within_parent
 from
    store_sales
   ,date_dim       d1
   ,store
 where
    d1.d_month_seq between 1200 and 1200+11
 and d1.d_date_sk = ss_sold_date_sk
 and s_store_sk  = ss_store_sk
 and s_state in
             ( select s_state
               from  (select store.s_state AS s_state,
  rank() over ( partition by store.s_state order by ((sum(store_sales.ss_net_profit)) IS NOT NULL), sum(store_sales.ss_net_profit) DESC ) AS ranking
 from   store_sales, store, date_dim
                      where  d_month_seq between 1200 and 1200+11
                and d_date_sk = ss_sold_date_sk
                and s_store_sk  = ss_store_sk
                      group by store.s_state
 ) tmp1
               where ranking <= 5
             )
 group by  rollup(store.s_state,store.s_county)

 order by ((lochierarchy) IS NOT NULL), lochierarchy DESC, ((case when lochierarchy = 0 then s_state  else null end) IS NULL), case when lochierarchy = 0 then s_state  else null end, ((rank_within_parent) IS NULL), rank_within_parent limit 100;

--= query_71
PRAGMA AnsiImplicitCrossJoin;
select item.i_brand_id AS brand_id,
  item.i_brand AS brand,
  time_dim.t_hour AS t_hour,
  time_dim.t_minute AS t_minute,
  sum(ext_price) AS ext_price
 from item, (select ws_ext_sales_price as ext_price,
                        ws_sold_date_sk as sold_date_sk,
                        ws_item_sk as sold_item_sk,
                        ws_sold_time_sk as time_sk
                 from web_sales,date_dim
                 where d_date_sk = ws_sold_date_sk
                   and d_moy=11
                   and d_year=1999
                 union all
                 select cs_ext_sales_price as ext_price,
                        cs_sold_date_sk as sold_date_sk,
                        cs_item_sk as sold_item_sk,
                        cs_sold_time_sk as time_sk
                 from catalog_sales,date_dim
                 where d_date_sk = cs_sold_date_sk
                   and d_moy=11
                   and d_year=1999
                 union all
                 select ss_ext_sales_price as ext_price,
                        ss_sold_date_sk as sold_date_sk,
                        ss_item_sk as sold_item_sk,
                        ss_sold_time_sk as time_sk
                 from store_sales,date_dim
                 where d_date_sk = ss_sold_date_sk
                   and d_moy=11
                   and d_year=1999
                 ) tmp,time_dim
 where
   sold_item_sk = i_item_sk
   and i_manager_id=1
   and time_sk = t_time_sk
   and (t_meal_time = 'breakfast' or t_meal_time = 'dinner')
 group by item.i_brand, item.i_brand_id, time_dim.t_hour, time_dim.t_minute
 order by ((ext_price) IS NOT NULL), ext_price DESC, ((brand_id) IS NULL), brand_id;

--= query_72
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_desc AS i_item_desc,
  warehouse.w_warehouse_name AS w_warehouse_name,
  d1.d_week_seq AS d_week_seq,
  sum(case when promotion.p_promo_sk is null then 1 else 0 end) AS no_promo,
  sum(case when promotion.p_promo_sk is not null then 1 else 0 end) AS promo,
  count(*) AS total_cnt
 from catalog_sales
join inventory on (catalog_sales.cs_item_sk = inventory.inv_item_sk)
join warehouse on (warehouse.w_warehouse_sk=inventory.inv_warehouse_sk)
join item on (item.i_item_sk = catalog_sales.cs_item_sk)
join customer_demographics on (catalog_sales.cs_bill_cdemo_sk = customer_demographics.cd_demo_sk)
join household_demographics on (catalog_sales.cs_bill_hdemo_sk = household_demographics.hd_demo_sk)
join date_dim d1 on (catalog_sales.cs_sold_date_sk = d1.d_date_sk)
join date_dim d2 on (inventory.inv_date_sk = d2.d_date_sk)
join date_dim d3 on (catalog_sales.cs_ship_date_sk = d3.d_date_sk)
left outer join promotion on (catalog_sales.cs_promo_sk=promotion.p_promo_sk)
left outer join catalog_returns on (catalog_returns.cr_item_sk = catalog_sales.cs_item_sk and catalog_returns.cr_order_number = catalog_sales.cs_order_number)
where d1.d_week_seq = d2.d_week_seq
  and inv_quantity_on_hand < cs_quantity
  and d3.d_date > cast(cast(d1.d_date as Date) + DateTime::IntervalFromDays(5) as String)
  and hd_buy_potential = '>10000'
  and d1.d_year = 1999
  and cd_marital_status = 'D'
group by item.i_item_desc, warehouse.w_warehouse_name, d1.d_week_seq
 order by ((total_cnt) IS NOT NULL), total_cnt DESC, ((i_item_desc) IS NULL), i_item_desc, ((w_warehouse_name) IS NULL), w_warehouse_name, ((d_week_seq) IS NULL), d_week_seq limit 100;

--= query_73
PRAGMA AnsiImplicitCrossJoin;
select c_last_name
       ,c_first_name
       ,c_salutation
       ,c_preferred_cust_flag
       ,ss_ticket_number
       ,cnt from
   (select store_sales.ss_ticket_number AS ss_ticket_number,
  store_sales.ss_customer_sk AS ss_customer_sk,
  count(*) AS cnt
 from store_sales,date_dim,store,household_demographics
    where store_sales.ss_sold_date_sk = date_dim.d_date_sk
    and store_sales.ss_store_sk = store.s_store_sk
    and store_sales.ss_hdemo_sk = household_demographics.hd_demo_sk
    and date_dim.d_dom between 1 and 2
    and (household_demographics.hd_buy_potential = '>10000' or
         household_demographics.hd_buy_potential = 'Unknown')
    and household_demographics.hd_vehicle_count > 0
    and case when household_demographics.hd_vehicle_count > 0 then
             household_demographics.hd_dep_count/ household_demographics.hd_vehicle_count else null end > 1
    and date_dim.d_year in (1999,1999+1,1999+2)
    and store.s_county in ('Williamson County','Franklin Parish','Bronx County','Orange County')
    group by store_sales.ss_ticket_number, store_sales.ss_customer_sk
 ) dj,customer
    where ss_customer_sk = c_customer_sk
      and cnt between 1 and 5
    order by ((cnt) IS NOT NULL), cnt DESC, ((c_last_name) IS NULL), c_last_name ASC;

--= query_74
PRAGMA AnsiImplicitCrossJoin;
$year_total = (select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  date_dim.d_year AS year,
  sum(store_sales.ss_net_paid) AS year_total,
  's' AS sale_type
 from customer
     ,store_sales
     ,date_dim
 where c_customer_sk = ss_customer_sk
   and ss_sold_date_sk = d_date_sk
   and d_year in (2001,2001+1)
 group by customer.c_customer_id, customer.c_first_name, customer.c_last_name, date_dim.d_year
 union all
 select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  date_dim.d_year AS year,
  sum(web_sales.ws_net_paid) AS year_total,
  'w' AS sale_type
 from customer
     ,web_sales
     ,date_dim
 where c_customer_sk = ws_bill_customer_sk
   and ws_sold_date_sk = d_date_sk
   and d_year in (2001,2001+1)
 group by customer.c_customer_id, customer.c_first_name, customer.c_last_name, date_dim.d_year
 );

  select
        t_s_secyear.customer_id AS customer_id, t_s_secyear.customer_first_name AS customer_first_name, t_s_secyear.customer_last_name AS customer_last_name
 from $year_total AS t_s_firstyear
     ,$year_total AS t_s_secyear
     ,$year_total AS t_w_firstyear
     ,$year_total AS t_w_secyear
 where t_s_secyear.customer_id = t_s_firstyear.customer_id
         and t_s_firstyear.customer_id = t_w_secyear.customer_id
         and t_s_firstyear.customer_id = t_w_firstyear.customer_id
         and t_s_firstyear.sale_type = 's'
         and t_w_firstyear.sale_type = 'w'
         and t_s_secyear.sale_type = 's'
         and t_w_secyear.sale_type = 'w'
         and t_s_firstyear.year = 2001
         and t_s_secyear.year = 2001+1
         and t_w_firstyear.year = 2001
         and t_w_secyear.year = 2001+1
         and t_s_firstyear.year_total > 0
         and t_w_firstyear.year_total > 0
         and case when t_w_firstyear.year_total > 0 then t_w_secyear.year_total / t_w_firstyear.year_total else null end
           > case when t_s_firstyear.year_total > 0 then t_s_secyear.year_total / t_s_firstyear.year_total else null end
 order by ((customer_id) IS NULL), customer_id limit 100;

--= query_75
PRAGMA AnsiImplicitCrossJoin;
$all_sales = (SELECT d_year AS d_year,
  i_brand_id AS i_brand_id,
  i_class_id AS i_class_id,
  i_category_id AS i_category_id,
  i_manufact_id AS i_manufact_id,
  SUM(sales_cnt) AS sales_cnt,
  SUM(sales_amt) AS sales_amt
 FROM (SELECT d_year
             ,i_brand_id
             ,i_class_id
             ,i_category_id
             ,i_manufact_id
             ,cs_quantity - COALESCE(cr_return_quantity,0) AS sales_cnt
             ,cs_ext_sales_price - COALESCE(cr_return_amount,0.0) AS sales_amt
       FROM catalog_sales JOIN item ON item.i_item_sk=catalog_sales.cs_item_sk
                          JOIN date_dim ON date_dim.d_date_sk=catalog_sales.cs_sold_date_sk
                          LEFT JOIN catalog_returns ON (catalog_sales.cs_order_number=catalog_returns.cr_order_number
                                                    AND catalog_sales.cs_item_sk=catalog_returns.cr_item_sk)
       WHERE i_category='Books'
       UNION
       SELECT d_year
             ,i_brand_id
             ,i_class_id
             ,i_category_id
             ,i_manufact_id
             ,ss_quantity - COALESCE(sr_return_quantity,0) AS sales_cnt
             ,ss_ext_sales_price - COALESCE(sr_return_amt,0.0) AS sales_amt
       FROM store_sales JOIN item ON item.i_item_sk=store_sales.ss_item_sk
                        JOIN date_dim ON date_dim.d_date_sk=store_sales.ss_sold_date_sk
                        LEFT JOIN store_returns ON (store_sales.ss_ticket_number=store_returns.sr_ticket_number
                                                AND store_sales.ss_item_sk=store_returns.sr_item_sk)
       WHERE i_category='Books'
       UNION
       SELECT d_year
             ,i_brand_id
             ,i_class_id
             ,i_category_id
             ,i_manufact_id
             ,ws_quantity - COALESCE(wr_return_quantity,0) AS sales_cnt
             ,ws_ext_sales_price - COALESCE(wr_return_amt,0.0) AS sales_amt
       FROM web_sales JOIN item ON item.i_item_sk=web_sales.ws_item_sk
                      JOIN date_dim ON date_dim.d_date_sk=web_sales.ws_sold_date_sk
                      LEFT JOIN web_returns ON (web_sales.ws_order_number=web_returns.wr_order_number
                                            AND web_sales.ws_item_sk=web_returns.wr_item_sk)
       WHERE i_category='Books') sales_detail
 GROUP BY d_year, i_brand_id, i_class_id, i_category_id, i_manufact_id
 );

 SELECT  prev_yr.d_year AS prev_year
                          ,curr_yr.d_year AS year
                          ,curr_yr.i_brand_id AS i_brand_id
                          ,curr_yr.i_class_id AS i_class_id
                          ,curr_yr.i_category_id AS i_category_id
                          ,curr_yr.i_manufact_id AS i_manufact_id
                          ,prev_yr.sales_cnt AS prev_yr_cnt
                          ,curr_yr.sales_cnt AS curr_yr_cnt
                          ,curr_yr.sales_cnt-prev_yr.sales_cnt AS sales_cnt_diff
                          ,curr_yr.sales_amt-prev_yr.sales_amt AS sales_amt_diff
 FROM $all_sales AS curr_yr, $all_sales AS prev_yr
 WHERE curr_yr.i_brand_id=prev_yr.i_brand_id
   AND curr_yr.i_class_id=prev_yr.i_class_id
   AND curr_yr.i_category_id=prev_yr.i_category_id
   AND curr_yr.i_manufact_id=prev_yr.i_manufact_id
   AND curr_yr.d_year=2002
   AND prev_yr.d_year=2002-1
   AND CAST(curr_yr.sales_cnt as Double)/CAST(prev_yr.sales_cnt as Double)<0.9
 ORDER BY ((sales_cnt_diff) IS NULL), sales_cnt_diff, ((sales_amt_diff) IS NULL), sales_amt_diff limit 100;

--= query_76
PRAGMA AnsiImplicitCrossJoin;
select channel AS channel,
  col_name AS col_name,
  d_year AS d_year,
  d_qoy AS d_qoy,
  i_category AS i_category,
  COUNT(*) AS sales_cnt,
  SUM(ext_sales_price) AS sales_amt
 FROM (
        SELECT 'store' as channel, 'ss_store_sk' col_name, d_year, d_qoy, i_category, ss_ext_sales_price ext_sales_price
         FROM store_sales, item, date_dim
         WHERE ss_store_sk IS NULL
           AND ss_sold_date_sk=d_date_sk
           AND ss_item_sk=i_item_sk
        UNION ALL
        SELECT 'web' as channel, 'ws_ship_customer_sk' col_name, d_year, d_qoy, i_category, ws_ext_sales_price ext_sales_price
         FROM web_sales, item, date_dim
         WHERE ws_ship_customer_sk IS NULL
           AND ws_sold_date_sk=d_date_sk
           AND ws_item_sk=i_item_sk
        UNION ALL
        SELECT 'catalog' as channel, 'cs_ship_addr_sk' col_name, d_year, d_qoy, i_category, cs_ext_sales_price ext_sales_price
         FROM catalog_sales, item, date_dim
         WHERE cs_ship_addr_sk IS NULL
           AND cs_sold_date_sk=d_date_sk
           AND cs_item_sk=i_item_sk) foo
GROUP BY channel, col_name, d_year, d_qoy, i_category
 ORDER BY ((channel) IS NULL), channel, ((col_name) IS NULL), col_name, ((d_year) IS NULL), d_year, ((d_qoy) IS NULL), d_qoy, ((i_category) IS NULL), i_category limit 100;

--= query_77
PRAGMA AnsiImplicitCrossJoin;
$ss = (select store.s_store_sk AS s_store_sk,
  sum(store_sales.ss_ext_sales_price) AS sales,
  sum(store_sales.ss_net_profit) AS profit
 from store_sales,
      date_dim,
      store
 where ss_sold_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
       and ss_store_sk = s_store_sk
 group by store.s_store_sk
 );
$sr = (select store.s_store_sk AS s_store_sk,
  sum(store_returns.sr_return_amt) AS returns,
  sum(store_returns.sr_net_loss) AS profit_loss
 from store_returns,
      date_dim,
      store
 where sr_returned_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
       and sr_store_sk = s_store_sk
 group by store.s_store_sk
 );
$cs = (select catalog_sales.cs_call_center_sk AS cs_call_center_sk,
  sum(catalog_sales.cs_ext_sales_price) AS sales,
  sum(catalog_sales.cs_net_profit) AS profit
 from catalog_sales,
      date_dim
 where cs_sold_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
 group by catalog_sales.cs_call_center_sk
 );
$cr = (select catalog_returns.cr_call_center_sk AS cr_call_center_sk,
  sum(catalog_returns.cr_return_amount) AS returns,
  sum(catalog_returns.cr_net_loss) AS profit_loss
 from catalog_returns,
      date_dim
 where cr_returned_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
 group by catalog_returns.cr_call_center_sk
 );
$ws = (select web_page.wp_web_page_sk AS wp_web_page_sk,
  sum(web_sales.ws_ext_sales_price) AS sales,
  sum(web_sales.ws_net_profit) AS profit
 from web_sales,
      date_dim,
      web_page
 where ws_sold_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
       and ws_web_page_sk = wp_web_page_sk
 group by web_page.wp_web_page_sk
 );
$wr = (select web_page.wp_web_page_sk AS wp_web_page_sk,
  sum(web_returns.wr_return_amt) AS returns,
  sum(web_returns.wr_net_loss) AS profit_loss
 from web_returns,
      date_dim,
      web_page
 where wr_returned_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
       and wr_web_page_sk = wp_web_page_sk
 group by web_page.wp_web_page_sk
 );

  select channel AS channel,
  id AS id,
  sum(sales) AS sales,
  sum(returns) AS returns,
  sum(profit) AS profit
 from
 (select 'store channel' as channel
        , ss.s_store_sk as id
        , sales
        , coalesce(returns, 0) as returns
        , (profit - coalesce(profit_loss,0)) as profit
 from   $ss AS ss left join $sr AS sr
        on  ss.s_store_sk = sr.s_store_sk
 union all
 select 'catalog channel' as channel
        , cs_call_center_sk as id
        , sales
        , returns
        , (profit - profit_loss) as profit
 from  $cs AS cs
       , $cr AS cr
 union all
 select 'web channel' as channel
        , ws.wp_web_page_sk as id
        , sales
        , coalesce(returns, 0) returns
        , (profit - coalesce(profit_loss,0)) as profit
 from   $ws AS ws left join $wr AS wr
        on  ws.wp_web_page_sk = wr.wp_web_page_sk
 ) x
 group by  rollup (channel, id)

 order by ((channel) IS NULL), channel, ((id) IS NULL), id limit 100;

--= query_78
PRAGMA AnsiImplicitCrossJoin;
$ws = (select date_dim.d_year AS ws_sold_year,
  web_sales.ws_item_sk AS ws_item_sk,
  web_sales.ws_bill_customer_sk AS ws_customer_sk,
  sum(web_sales.ws_quantity) AS ws_qty,
  sum(web_sales.ws_wholesale_cost) AS ws_wc,
  sum(web_sales.ws_sales_price) AS ws_sp
 from web_sales
   left join web_returns on web_returns.wr_order_number=web_sales.ws_order_number and web_sales.ws_item_sk=web_returns.wr_item_sk
   join date_dim on web_sales.ws_sold_date_sk = date_dim.d_date_sk
   where wr_order_number is null
   group by date_dim.d_year, web_sales.ws_item_sk, web_sales.ws_bill_customer_sk
 );
$cs = (select date_dim.d_year AS cs_sold_year,
  catalog_sales.cs_item_sk AS cs_item_sk,
  catalog_sales.cs_bill_customer_sk AS cs_customer_sk,
  sum(catalog_sales.cs_quantity) AS cs_qty,
  sum(catalog_sales.cs_wholesale_cost) AS cs_wc,
  sum(catalog_sales.cs_sales_price) AS cs_sp
 from catalog_sales
   left join catalog_returns on catalog_returns.cr_order_number=catalog_sales.cs_order_number and catalog_sales.cs_item_sk=catalog_returns.cr_item_sk
   join date_dim on catalog_sales.cs_sold_date_sk = date_dim.d_date_sk
   where cr_order_number is null
   group by date_dim.d_year, catalog_sales.cs_item_sk, catalog_sales.cs_bill_customer_sk
 );
$ss = (select date_dim.d_year AS ss_sold_year,
  store_sales.ss_item_sk AS ss_item_sk,
  store_sales.ss_customer_sk AS ss_customer_sk,
  sum(store_sales.ss_quantity) AS ss_qty,
  sum(store_sales.ss_wholesale_cost) AS ss_wc,
  sum(store_sales.ss_sales_price) AS ss_sp
 from store_sales
   left join store_returns on store_returns.sr_ticket_number=store_sales.ss_ticket_number and store_sales.ss_item_sk=store_returns.sr_item_sk
   join date_dim on store_sales.ss_sold_date_sk = date_dim.d_date_sk
   where sr_ticket_number is null
   group by date_dim.d_year, store_sales.ss_item_sk, store_sales.ss_customer_sk
 );

 select
ss_customer_sk,
Math::Round(cast(ss_qty as Double)/(coalesce(ws_qty,0)+coalesce(cs_qty,0)), -(2)) ratio,
ss_qty store_qty, ss_wc store_wholesale_cost, ss_sp store_sales_price,
coalesce(ws_qty,0)+coalesce(cs_qty,0) other_chan_qty,
coalesce(ws_wc,0)+coalesce(cs_wc,0) other_chan_wholesale_cost,
coalesce(ws_sp,0)+coalesce(cs_sp,0) other_chan_sales_price
from $ss AS ss
left join $ws AS ws on (ws.ws_sold_year=ss.ss_sold_year and ws.ws_item_sk=ss.ss_item_sk and ws.ws_customer_sk=ss.ss_customer_sk)
left join $cs AS cs on (cs.cs_sold_year=ss.ss_sold_year and cs.cs_item_sk=ss.ss_item_sk and cs.cs_customer_sk=ss.ss_customer_sk)
where (coalesce(ws_qty,0)>0 or coalesce(cs_qty, 0)>0) and ss_sold_year=2000
order by ((ss_customer_sk) IS NULL), ss_customer_sk, ((ss_qty) IS NOT NULL), ss_qty DESC, ((ss_wc) IS NOT NULL), ss_wc DESC, ((ss_sp) IS NOT NULL), ss_sp DESC, ((other_chan_qty) IS NULL), other_chan_qty, ((other_chan_wholesale_cost) IS NULL), other_chan_wholesale_cost, ((other_chan_sales_price) IS NULL), other_chan_sales_price, ((ratio) IS NULL), ratio limit 100;

--= query_79
PRAGMA AnsiImplicitCrossJoin;
select
  c_last_name,c_first_name,Unicode::Substring(s_city, CAST(0 AS Uint32), CAST(30 AS Uint32)),ss_ticket_number,amt,profit
  from
   (select store_sales.ss_ticket_number AS ss_ticket_number,
  store_sales.ss_customer_sk AS ss_customer_sk,
  store.s_city AS s_city,
  sum(store_sales.ss_coupon_amt) AS amt,
  sum(store_sales.ss_net_profit) AS profit
 from store_sales,date_dim,store,household_demographics
    where store_sales.ss_sold_date_sk = date_dim.d_date_sk
    and store_sales.ss_store_sk = store.s_store_sk
    and store_sales.ss_hdemo_sk = household_demographics.hd_demo_sk
    and (household_demographics.hd_dep_count = 6 or household_demographics.hd_vehicle_count > -1)
    and date_dim.d_dow = 1
    and date_dim.d_year in (1999,1999+1,1999+2)
    and store.s_number_employees between 200 and 295
    group by store_sales.ss_ticket_number, store_sales.ss_customer_sk, store_sales.ss_addr_sk, store.s_city
 ) ms,customer
    where ss_customer_sk = c_customer_sk
 order by ((c_last_name) IS NULL), c_last_name, ((c_first_name) IS NULL), c_first_name, ((Unicode::Substring(s_city, CAST(0 AS Uint32), CAST(30 AS Uint32))) IS NULL), Unicode::Substring(s_city, CAST(0 AS Uint32), CAST(30 AS Uint32)), ((profit) IS NULL), profit limit 100;

--= query_80
PRAGMA AnsiImplicitCrossJoin;
$ssr = (select store.s_store_id AS store_id,
  sum(store_sales.ss_ext_sales_price) AS sales,
  sum(coalesce(store_returns.sr_return_amt, 0)) AS returns,
  sum(store_sales.ss_net_profit - coalesce(store_returns.sr_net_loss, 0)) AS profit
 from store_sales left outer join store_returns on
         (store_sales.ss_item_sk = store_returns.sr_item_sk and store_sales.ss_ticket_number = store_returns.sr_ticket_number),
     date_dim,
     store,
     item,
     promotion
 where ss_sold_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
       and ss_store_sk = s_store_sk
       and ss_item_sk = i_item_sk
       and i_current_price > 50
       and ss_promo_sk = p_promo_sk
       and p_channel_tv = 'N'
 group by store.s_store_id
 );
$csr = (select catalog_page.cp_catalog_page_id AS catalog_page_id,
  sum(catalog_sales.cs_ext_sales_price) AS sales,
  sum(coalesce(catalog_returns.cr_return_amount, 0)) AS returns,
  sum(catalog_sales.cs_net_profit - coalesce(catalog_returns.cr_net_loss, 0)) AS profit
 from catalog_sales left outer join catalog_returns on
         (catalog_sales.cs_item_sk = catalog_returns.cr_item_sk and catalog_sales.cs_order_number = catalog_returns.cr_order_number),
     date_dim,
     catalog_page,
     item,
     promotion
 where cs_sold_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
        and cs_catalog_page_sk = cp_catalog_page_sk
       and cs_item_sk = i_item_sk
       and i_current_price > 50
       and cs_promo_sk = p_promo_sk
       and p_channel_tv = 'N'
group by catalog_page.cp_catalog_page_id
 );
$wsr = (select web_site.web_site_id AS web_site_id,
  sum(web_sales.ws_ext_sales_price) AS sales,
  sum(coalesce(web_returns.wr_return_amt, 0)) AS returns,
  sum(web_sales.ws_net_profit - coalesce(web_returns.wr_net_loss, 0)) AS profit
 from web_sales left outer join web_returns on
         (web_sales.ws_item_sk = web_returns.wr_item_sk and web_sales.ws_order_number = web_returns.wr_order_number),
     date_dim,
     web_site,
     item,
     promotion
 where ws_sold_date_sk = d_date_sk
       and d_date between ('2000-08-23')
                  and '2000-09-22'
        and ws_web_site_sk = web_site_sk
       and ws_item_sk = i_item_sk
       and i_current_price > 50
       and ws_promo_sk = p_promo_sk
       and p_channel_tv = 'N'
group by web_site.web_site_id
 );

  select channel AS channel,
  id AS id,
  sum(sales) AS sales,
  sum(returns) AS returns,
  sum(profit) AS profit
 from
 (select 'store channel' as channel
        , 'store' || store_id as id
        , sales
        , returns
        , profit
 from   $ssr AS ssr
 union all
 select 'catalog channel' as channel
        , 'catalog_page' || catalog_page_id as id
        , sales
        , returns
        , profit
 from  $csr AS csr
 union all
 select 'web channel' as channel
        , 'web_site' || web_site_id as id
        , sales
        , returns
        , profit
 from   $wsr AS wsr
 ) x
 group by  rollup (channel, id)

 order by ((channel) IS NULL), channel, ((id) IS NULL), id limit 100;

--= query_81
PRAGMA AnsiImplicitCrossJoin;
$customer_total_return = (select catalog_returns.cr_returning_customer_sk AS ctr_customer_sk,
  customer_address.ca_state AS ctr_state,
  sum(catalog_returns.cr_return_amt_inc_tax) AS ctr_total_return
 from catalog_returns
     ,date_dim
     ,customer_address
 where cr_returned_date_sk = d_date_sk
   and d_year =2000
   and cr_returning_addr_sk = ca_address_sk
 group by catalog_returns.cr_returning_customer_sk, customer_address.ca_state
 );
$ctr_state_avg = (select ctr_state AS ctr_state,
  avg(ctr_total_return)*1.2 AS ctr_avg
 from $customer_total_return AS customer_total_return group by ctr_state
 );

  select  c_customer_id,c_salutation,c_first_name,c_last_name,ca_street_number,ca_street_name
                   ,ca_street_type,ca_suite_number,ca_city,ca_county,ca_state,ca_zip,ca_country,ca_gmt_offset
                  ,ca_location_type,ctr_total_return
 from $customer_total_return AS ctr1
     ,customer_address
     ,customer
     ,$ctr_state_avg AS ctr_state_avg
 where ctr1.ctr_total_return > ctr_state_avg.ctr_avg
              and ctr_state_avg.ctr_state = ctr1.ctr_state
       and ca_address_sk = c_current_addr_sk
       and ca_state = 'GA'
       and ctr1.ctr_customer_sk = c_customer_sk
 order by ((c_customer_id) IS NULL), c_customer_id, ((c_salutation) IS NULL), c_salutation, ((c_first_name) IS NULL), c_first_name, ((c_last_name) IS NULL), c_last_name, ((ca_street_number) IS NULL), ca_street_number, ((ca_street_name) IS NULL), ca_street_name, ((ca_street_type) IS NULL), ca_street_type, ((ca_suite_number) IS NULL), ca_suite_number, ((ca_city) IS NULL), ca_city, ((ca_county) IS NULL), ca_county, ((ca_state) IS NULL), ca_state, ((ca_zip) IS NULL), ca_zip, ((ca_country) IS NULL), ca_country, ((ca_gmt_offset) IS NULL), ca_gmt_offset, ((ca_location_type) IS NULL), ca_location_type, ((ctr_total_return) IS NULL), ctr_total_return limit 100;

--= query_82
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  item.i_current_price AS i_current_price
 from item, inventory, date_dim, store_sales
 where i_current_price between 62 and 62+30
 and inv_item_sk = i_item_sk
 and d_date_sk=inv_date_sk
 and d_date between ('2000-05-25') and '2000-07-24'
 and i_manufact_id in (129,270,821,423)
 and inv_quantity_on_hand between 100 and 500
 and ss_item_sk = i_item_sk
 group by item.i_item_id, item.i_item_desc, item.i_current_price
 order by ((i_item_id) IS NULL), i_item_id limit 100;

--= query_83
PRAGMA AnsiImplicitCrossJoin;
$sr_items = (select item.i_item_id AS item_id,
  sum(store_returns.sr_return_quantity) AS sr_item_qty
 from store_returns,
      item,
      date_dim
 where sr_item_sk = i_item_sk
 and   d_date    in
    (select d_date
    from date_dim
    where d_week_seq in
        (select d_week_seq
        from date_dim
      where d_date in ('2000-06-30','2000-09-27','2000-11-17')))
 and   sr_returned_date_sk   = d_date_sk
 group by item.i_item_id
 );
$cr_items = (select item.i_item_id AS item_id,
  sum(catalog_returns.cr_return_quantity) AS cr_item_qty
 from catalog_returns,
      item,
      date_dim
 where cr_item_sk = i_item_sk
 and   d_date    in
    (select d_date
    from date_dim
    where d_week_seq in
        (select d_week_seq
        from date_dim
      where d_date in ('2000-06-30','2000-09-27','2000-11-17')))
 and   cr_returned_date_sk   = d_date_sk
 group by item.i_item_id
 );
$wr_items = (select item.i_item_id AS item_id,
  sum(web_returns.wr_return_quantity) AS wr_item_qty
 from web_returns,
      item,
      date_dim
 where wr_item_sk = i_item_sk
 and   d_date    in
    (select d_date
    from date_dim
    where d_week_seq in
        (select d_week_seq
        from date_dim
        where d_date in ('2000-06-30','2000-09-27','2000-11-17')))
 and   wr_returned_date_sk   = d_date_sk
 group by item.i_item_id
 );

  select  sr_items.item_id AS item_id
       ,sr_item_qty
       ,sr_item_qty/(sr_item_qty+cr_item_qty+wr_item_qty)/3.0 * 100 sr_dev
       ,cr_item_qty
       ,cr_item_qty/(sr_item_qty+cr_item_qty+wr_item_qty)/3.0 * 100 cr_dev
       ,wr_item_qty
       ,wr_item_qty/(sr_item_qty+cr_item_qty+wr_item_qty)/3.0 * 100 wr_dev
       ,(sr_item_qty+cr_item_qty+wr_item_qty)/3.0 average
 from $sr_items AS sr_items
     ,$cr_items AS cr_items
     ,$wr_items AS wr_items
 where sr_items.item_id=cr_items.item_id
   and sr_items.item_id=wr_items.item_id
 order by ((sr_items.item_id) IS NULL), sr_items.item_id, ((sr_item_qty) IS NULL), sr_item_qty limit 100;

--= query_84
PRAGMA AnsiImplicitCrossJoin;
select  c_customer_id as customer_id
       , coalesce(c_last_name,'') || ', ' || coalesce(c_first_name,'') as customername
 from customer
     ,customer_address
     ,customer_demographics
     ,household_demographics
     ,income_band
     ,store_returns
 where ca_city          =  'Edgewood'
   and c_current_addr_sk = ca_address_sk
   and ib_lower_bound   >=  38128
   and ib_upper_bound   <=  38128 + 50000
   and ib_income_band_sk = hd_income_band_sk
   and cd_demo_sk = c_current_cdemo_sk
   and hd_demo_sk = c_current_hdemo_sk
   and sr_cdemo_sk = cd_demo_sk
 order by ((c_customer_id) IS NULL), c_customer_id limit 100;

--= query_85
PRAGMA AnsiImplicitCrossJoin;
select Unicode::Substring(reason.r_reason_desc, CAST(0 AS Uint32), CAST(20 AS Uint32)) AS c13,
  avg(web_sales.ws_quantity) AS c14,
  avg(web_returns.wr_refunded_cash) AS c15,
  avg(web_returns.wr_fee) AS c16
 from web_sales, web_returns, web_page, customer_demographics cd1,
      customer_demographics cd2, customer_address, date_dim, reason
 where ws_web_page_sk = wp_web_page_sk
   and ws_item_sk = wr_item_sk
   and ws_order_number = wr_order_number
   and ws_sold_date_sk = d_date_sk and d_year = 2000
   and cd1.cd_demo_sk = wr_refunded_cdemo_sk
   and cd2.cd_demo_sk = wr_returning_cdemo_sk
   and ca_address_sk = wr_refunded_addr_sk
   and r_reason_sk = wr_reason_sk
   and
   (
    (
     cd1.cd_marital_status = 'M'
     and
     cd1.cd_marital_status = cd2.cd_marital_status
     and
     cd1.cd_education_status = 'Advanced Degree'
     and
     cd1.cd_education_status = cd2.cd_education_status
     and
     ws_sales_price between 100.00 and 150.00
    )
   or
    (
     cd1.cd_marital_status = 'S'
     and
     cd1.cd_marital_status = cd2.cd_marital_status
     and
     cd1.cd_education_status = 'College'
     and
     cd1.cd_education_status = cd2.cd_education_status
     and
     ws_sales_price between 50.00 and 100.00
    )
   or
    (
     cd1.cd_marital_status = 'W'
     and
     cd1.cd_marital_status = cd2.cd_marital_status
     and
     cd1.cd_education_status = '2 yr Degree'
     and
     cd1.cd_education_status = cd2.cd_education_status
     and
     ws_sales_price between 150.00 and 200.00
    )
   )
   and
   (
    (
     ca_country = 'United States'
     and
     ca_state in ('IN', 'OH', 'NJ')
     and ws_net_profit between 100 and 200
    )
    or
    (
     ca_country = 'United States'
     and
     ca_state in ('WI', 'CT', 'KY')
     and ws_net_profit between 150 and 300
    )
    or
    (
     ca_country = 'United States'
     and
     ca_state in ('LA', 'IA', 'AR')
     and ws_net_profit between 50 and 250
    )
   )
group by reason.r_reason_desc
 order by ((c13) IS NULL), c13, ((c14) IS NULL), c14, ((c15) IS NULL), c15, ((c16) IS NULL), c16 limit 100;

--= query_86
PRAGMA AnsiImplicitCrossJoin;
select sum(web_sales.ws_net_paid) AS total_sum,
  item.i_category AS i_category,
  item.i_class AS i_class,
  grouping(item.i_category)+grouping(item.i_class) AS lochierarchy,
  rank() over (
    partition by grouping(item.i_category)+grouping(item.i_class),
    case when grouping(item.i_class) = 0 then item.i_category  else null end
    order by ((sum(web_sales.ws_net_paid)) IS NOT NULL), sum(web_sales.ws_net_paid) DESC ) AS rank_within_parent
 from
    web_sales
   ,date_dim       d1
   ,item
 where
    d1.d_month_seq between 1200 and 1200+11
 and d1.d_date_sk = ws_sold_date_sk
 and i_item_sk  = ws_item_sk
 group by  rollup(item.i_category,item.i_class)

 order by ((lochierarchy) IS NOT NULL), lochierarchy DESC, ((case when lochierarchy = 0 then i_category  else null end) IS NULL), case when lochierarchy = 0 then i_category  else null end, ((rank_within_parent) IS NULL), rank_within_parent limit 100;

--= query_87
PRAGMA AnsiImplicitCrossJoin;
select count(*)
from ((select distinct c_last_name, c_first_name, d_date
       from store_sales, date_dim, customer
       where store_sales.ss_sold_date_sk = date_dim.d_date_sk
         and store_sales.ss_customer_sk = customer.c_customer_sk
         and d_month_seq between 1200 and 1200+11)
       except
      (select distinct c_last_name, c_first_name, d_date
       from catalog_sales, date_dim, customer
       where catalog_sales.cs_sold_date_sk = date_dim.d_date_sk
         and catalog_sales.cs_bill_customer_sk = customer.c_customer_sk
         and d_month_seq between 1200 and 1200+11)
       except
      (select distinct c_last_name, c_first_name, d_date
       from web_sales, date_dim, customer
       where web_sales.ws_sold_date_sk = date_dim.d_date_sk
         and web_sales.ws_bill_customer_sk = customer.c_customer_sk
         and d_month_seq between 1200 and 1200+11)
) cool_cust;

--= query_88
PRAGMA AnsiImplicitCrossJoin;
select h8_30_to_9, h9_to_9_30, h9_30_to_10, h10_to_10_30, h10_30_to_11, h11_to_11_30, h11_30_to_12, h12_to_12_30 from
 (select count(*) AS h8_30_to_9
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 8
     and time_dim.t_minute >= 30
     and ((household_demographics.hd_dep_count = 4 and household_demographics.hd_vehicle_count<=4+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2))
     and store.s_store_name = 'ese') s1,
 (select count(*) AS h9_to_9_30
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 9
     and time_dim.t_minute < 30
     and ((household_demographics.hd_dep_count = 4 and household_demographics.hd_vehicle_count<=4+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2))
     and store.s_store_name = 'ese') s2,
 (select count(*) AS h9_30_to_10
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 9
     and time_dim.t_minute >= 30
     and ((household_demographics.hd_dep_count = 4 and household_demographics.hd_vehicle_count<=4+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2))
     and store.s_store_name = 'ese') s3,
 (select count(*) AS h10_to_10_30
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 10
     and time_dim.t_minute < 30
     and ((household_demographics.hd_dep_count = 4 and household_demographics.hd_vehicle_count<=4+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2))
     and store.s_store_name = 'ese') s4,
 (select count(*) AS h10_30_to_11
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 10
     and time_dim.t_minute >= 30
     and ((household_demographics.hd_dep_count = 4 and household_demographics.hd_vehicle_count<=4+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2))
     and store.s_store_name = 'ese') s5,
 (select count(*) AS h11_to_11_30
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 11
     and time_dim.t_minute < 30
     and ((household_demographics.hd_dep_count = 4 and household_demographics.hd_vehicle_count<=4+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2))
     and store.s_store_name = 'ese') s6,
 (select count(*) AS h11_30_to_12
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 11
     and time_dim.t_minute >= 30
     and ((household_demographics.hd_dep_count = 4 and household_demographics.hd_vehicle_count<=4+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2))
     and store.s_store_name = 'ese') s7,
 (select count(*) AS h12_to_12_30
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 12
     and time_dim.t_minute < 30
     and ((household_demographics.hd_dep_count = 4 and household_demographics.hd_vehicle_count<=4+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2))
     and store.s_store_name = 'ese') s8;

--= query_89
PRAGMA AnsiImplicitCrossJoin;
select i_category, i_class, i_brand, s_store_name, s_company_name, d_moy, sum_sales, avg_monthly_sales from(
select item.i_category AS i_category,
  item.i_class AS i_class,
  item.i_brand AS i_brand,
  store.s_store_name AS s_store_name,
  store.s_company_name AS s_company_name,
  date_dim.d_moy AS d_moy,
  sum(store_sales.ss_sales_price) AS sum_sales,
  avg(sum(store_sales.ss_sales_price)) over
         (partition by item.i_category, item.i_brand, store.s_store_name, store.s_company_name) AS avg_monthly_sales
 from item, store_sales, date_dim, store
where ss_item_sk = i_item_sk and
      ss_sold_date_sk = d_date_sk and
      ss_store_sk = s_store_sk and
      d_year in (1999) and
        ((i_category in ('Books','Electronics','Sports') and
          i_class in ('computers','stereo','football')
         )
      or (i_category in ('Men','Jewelry','Women') and
          i_class in ('shirts','birdal','dresses')
        ))
group by item.i_category, item.i_class, item.i_brand, store.s_store_name, store.s_company_name, date_dim.d_moy
 ) tmp1
where case when (avg_monthly_sales <> 0) then (abs(sum_sales - avg_monthly_sales) / avg_monthly_sales) else null end > 0.1
order by ((sum_sales - avg_monthly_sales) IS NULL), sum_sales - avg_monthly_sales, ((s_store_name) IS NULL), s_store_name limit 100;

--= query_90
PRAGMA AnsiImplicitCrossJoin;
select  cast(amc as Double)/nullif(cast(pmc as Double),0) am_pm_ratio
 from ( select count(*) AS amc
 from web_sales, household_demographics , time_dim, web_page
       where ws_sold_time_sk = time_dim.t_time_sk
         and ws_ship_hdemo_sk = household_demographics.hd_demo_sk
         and ws_web_page_sk = web_page.wp_web_page_sk
         and time_dim.t_hour between 8 and 8+1
         and household_demographics.hd_dep_count = 6
         and web_page.wp_char_count between 5000 and 5200) at,
      ( select count(*) AS pmc
 from web_sales, household_demographics , time_dim, web_page
       where ws_sold_time_sk = time_dim.t_time_sk
         and ws_ship_hdemo_sk = household_demographics.hd_demo_sk
         and ws_web_page_sk = web_page.wp_web_page_sk
         and time_dim.t_hour between 19 and 19+1
         and household_demographics.hd_dep_count = 6
         and web_page.wp_char_count between 5000 and 5200) pt
 order by ((am_pm_ratio) IS NULL), am_pm_ratio limit 100;

--= query_91
PRAGMA AnsiImplicitCrossJoin;
select call_center.cc_call_center_id AS Call_Center,
  call_center.cc_name AS Call_Center_Name,
  call_center.cc_manager AS Manager,
  sum(catalog_returns.cr_net_loss) AS Returns_Loss
 from
        call_center,
        catalog_returns,
        date_dim,
        customer,
        customer_address,
        customer_demographics,
        household_demographics
where
        cr_call_center_sk       = cc_call_center_sk
and     cr_returned_date_sk     = d_date_sk
and     cr_returning_customer_sk= c_customer_sk
and     cd_demo_sk              = c_current_cdemo_sk
and     hd_demo_sk              = c_current_hdemo_sk
and     ca_address_sk           = c_current_addr_sk
and     d_year                  = 1998
and     d_moy                   = 11
and     ( (cd_marital_status       = 'M' and cd_education_status     = 'Unknown')
        or(cd_marital_status       = 'W' and cd_education_status     = 'Advanced Degree'))
and     hd_buy_potential like 'Unknown%'
and     ca_gmt_offset           = -7
group by call_center.cc_call_center_id, call_center.cc_name, call_center.cc_manager, customer_demographics.cd_marital_status, customer_demographics.cd_education_status
 order by ((Returns_Loss) IS NOT NULL), Returns_Loss DESC;

--= query_92
PRAGMA AnsiImplicitCrossJoin;
$disc = (select web_sales.ws_item_sk AS ditem,
  1.3 * avg(web_sales.ws_ext_discount_amt) AS avg_disc
 from web_sales, date_dim
  where d_date between '2000-01-27' and '2000-04-26'
    and d_date_sk = ws_sold_date_sk
  group by web_sales.ws_item_sk
 );

select sum(web_sales.ws_ext_discount_amt) AS `Excess Discount Amount`
 from web_sales, item, date_dim, $disc AS disc
where i_manufact_id = 350
  and i_item_sk = ws_item_sk
  and d_date between '2000-01-27' and '2000-04-26'
  and d_date_sk = ws_sold_date_sk
  and disc.ditem = web_sales.ws_item_sk
  and web_sales.ws_ext_discount_amt > disc.avg_disc
order by ((`Excess Discount Amount`) IS NULL), `Excess Discount Amount` limit 100;

--= query_93
PRAGMA AnsiImplicitCrossJoin;
select ss_customer_sk AS ss_customer_sk,
  sum(act_sales) AS sumsales
 from (select ss_item_sk
                  ,ss_ticket_number
                  ,ss_customer_sk
                  ,case when sr_return_quantity is not null then (ss_quantity-sr_return_quantity)*ss_sales_price
                                                            else (ss_quantity*ss_sales_price) end act_sales
            from store_sales left outer join store_returns on (store_returns.sr_item_sk = store_sales.ss_item_sk
                                                               and store_returns.sr_ticket_number = store_sales.ss_ticket_number)
                ,reason
            where sr_reason_sk = r_reason_sk
              and r_reason_desc = 'reason 28') t
      group by ss_customer_sk
 order by ((sumsales) IS NULL), sumsales, ((ss_customer_sk) IS NULL), ss_customer_sk limit 100;

--= query_94
PRAGMA AnsiImplicitCrossJoin;
select count(distinct ws1.ws_order_number) AS `order count`,
  sum(ws1.ws_ext_ship_cost) AS `total shipping cost`,
  sum(ws1.ws_net_profit) AS `total net profit`
 from web_sales ws1, date_dim, customer_address, web_site
where d_date between '1999-02-01' and '1999-04-02'
  and ws1.ws_ship_date_sk = d_date_sk
  and ws1.ws_ship_addr_sk = ca_address_sk
  and ca_state = 'IL'
  and ws1.ws_web_site_sk = web_site_sk
  and web_company_name = 'pri'
  and ws1.ws_order_number in (select ws_order_number AS ws_order_number
 from web_sales group by ws_order_number
 having count(distinct ws_warehouse_sk) > 1)
  and ws1.ws_order_number not in (select wr_order_number from web_returns)
order by ((`order count`) IS NULL), `order count` limit 100;

--= query_95
PRAGMA AnsiImplicitCrossJoin;
$ws_wh = (select ws1.ws_order_number AS ws_order_number,ws1.ws_warehouse_sk wh1,ws2.ws_warehouse_sk wh2
 from web_sales ws1,web_sales ws2
 where ws1.ws_order_number = ws2.ws_order_number
   and ws1.ws_warehouse_sk <> ws2.ws_warehouse_sk);

 select count(distinct ws1.ws_order_number) AS `order count`,
  sum(ws1.ws_ext_ship_cost) AS `total shipping cost`,
  sum(ws1.ws_net_profit) AS `total net profit`
 from
   web_sales ws1
  ,date_dim
  ,customer_address
  ,web_site
where
    d_date between '1999-02-01' and
           '1999-04-02'
and ws1.ws_ship_date_sk = d_date_sk
and ws1.ws_ship_addr_sk = ca_address_sk
and ca_state = 'IL'
and ws1.ws_web_site_sk = web_site_sk
and web_company_name = 'pri'
and ws1.ws_order_number in (select ws_order_number
                            from $ws_wh AS ws_wh)
and ws1.ws_order_number in (select wr_order_number
                            from web_returns,$ws_wh AS ws_wh
                            where wr_order_number = ws_wh.ws_order_number)
order by ((`order count`) IS NULL), `order count` limit 100;

--= query_96
PRAGMA AnsiImplicitCrossJoin;
select count(*) AS c17
 from store_sales
    ,household_demographics
    ,time_dim, store
where ss_sold_time_sk = time_dim.t_time_sk
    and ss_hdemo_sk = household_demographics.hd_demo_sk
    and ss_store_sk = s_store_sk
    and time_dim.t_hour = 20
    and time_dim.t_minute >= 30
    and household_demographics.hd_dep_count = 7
    and store.s_store_name = 'ese'
order by ((c17) IS NULL), c17 limit 100;

--= query_97
PRAGMA AnsiImplicitCrossJoin;
$ssci = (select store_sales.ss_customer_sk AS customer_sk,
  store_sales.ss_item_sk AS item_sk
 from store_sales,date_dim
where ss_sold_date_sk = d_date_sk
  and d_month_seq between 1200 and 1200 + 11
group by store_sales.ss_customer_sk, store_sales.ss_item_sk
 );
$csci = (select catalog_sales.cs_bill_customer_sk AS customer_sk,
  catalog_sales.cs_item_sk AS item_sk
 from catalog_sales,date_dim
where cs_sold_date_sk = d_date_sk
  and d_month_seq between 1200 and 1200 + 11
group by catalog_sales.cs_bill_customer_sk, catalog_sales.cs_item_sk
 );

 select sum(case when ssci.customer_sk is not null and csci.customer_sk is null then 1 else 0 end) AS store_only,
  sum(case when ssci.customer_sk is null and csci.customer_sk is not null then 1 else 0 end) AS catalog_only,
  sum(case when ssci.customer_sk is not null and csci.customer_sk is not null then 1 else 0 end) AS store_and_catalog
 from $ssci AS ssci full outer join $csci AS csci on (ssci.customer_sk=csci.customer_sk
                               and ssci.item_sk = csci.item_sk)
limit 100;

--= query_98
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  item.i_category AS i_category,
  item.i_class AS i_class,
  item.i_current_price AS i_current_price,
  sum(store_sales.ss_ext_sales_price) AS itemrevenue,
  sum(store_sales.ss_ext_sales_price)*100/sum(sum(store_sales.ss_ext_sales_price)) over
          (partition by item.i_class) AS revenueratio
 from
    store_sales
        ,item
        ,date_dim
where
    ss_item_sk = i_item_sk
    and i_category in ('Men', 'Sports', 'Jewelry')
    and ss_sold_date_sk = d_date_sk
    and d_date between ('1999-02-22')
                and '1999-03-24'
group by item.i_item_id, item.i_item_desc, item.i_category, item.i_class, item.i_current_price
 order by ((i_category) IS NULL), i_category, ((i_class) IS NULL), i_class, ((i_item_id) IS NULL), i_item_id, ((i_item_desc) IS NULL), i_item_desc, ((revenueratio) IS NULL), revenueratio;

--= query_99
PRAGMA AnsiImplicitCrossJoin;
select gk18,
  ship_mode.sm_type AS sm_type,
  call_center.cc_name AS cc_name,
  sum(case when (catalog_sales.cs_ship_date_sk - catalog_sales.cs_sold_date_sk <= 30 ) then 1 else 0 end) AS `30 days`,
  sum(case when (catalog_sales.cs_ship_date_sk - catalog_sales.cs_sold_date_sk > 30) and
                 (catalog_sales.cs_ship_date_sk - catalog_sales.cs_sold_date_sk <= 60) then 1 else 0 end ) AS `31-60 days`,
  sum(case when (catalog_sales.cs_ship_date_sk - catalog_sales.cs_sold_date_sk > 60) and
                 (catalog_sales.cs_ship_date_sk - catalog_sales.cs_sold_date_sk <= 90) then 1 else 0 end) AS `61-90 days`,
  sum(case when (catalog_sales.cs_ship_date_sk - catalog_sales.cs_sold_date_sk > 90) and
                 (catalog_sales.cs_ship_date_sk - catalog_sales.cs_sold_date_sk <= 120) then 1 else 0 end) AS `91-120 days`,
  sum(case when (catalog_sales.cs_ship_date_sk - catalog_sales.cs_sold_date_sk  > 120) then 1 else 0 end) AS `>120 days`
 from
   catalog_sales
  ,warehouse
  ,ship_mode
  ,call_center
  ,date_dim
where
    d_month_seq between 1200 and 1200 + 11
and cs_ship_date_sk   = d_date_sk
and cs_warehouse_sk   = w_warehouse_sk
and cs_ship_mode_sk   = sm_ship_mode_sk
and cs_call_center_sk = cc_call_center_sk
group by Unicode::Substring(warehouse.w_warehouse_name, CAST(0 AS Uint32), CAST(20 AS Uint32)) AS gk18, ship_mode.sm_type, call_center.cc_name
 order by ((gk18) IS NULL), gk18, ((sm_type) IS NULL), sm_type, ((cc_name) IS NULL), cc_name limit 100;
