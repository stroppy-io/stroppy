-- TPC-DS query workload for YDB (YQL). 99 queries (query_14/23/24/39 are two
-- parts _a/_b, so 103 named --= sections), derived from the canonical pg.sql at
-- the same qualification parameters. Every statement opens with PRAGMA
-- AnsiImplicitCrossJoin (TPC-DS uses comma joins). YQL rewrites vs pg.sql: ANSI
-- WITH CTEs become $named subqueries; multi-source GROUP BY / SELECT / ORDER BY
-- columns are correlation-qualified; correlated subqueries/EXISTS are
-- decorrelated (YQL has none); dates are Utf8 ISO strings (schema.ydb.sql), so
-- date literals drop the cast and date arithmetic is baked to a literal;
-- decimal casts become Double, substring becomes Unicode::Substring. See
-- workloads/tpcds/README.md.

--= query_1
PRAGMA AnsiImplicitCrossJoin;
$customer_total_return = (select store_returns.sr_customer_sk AS ctr_customer_sk,
  store_returns.sr_store_sk AS ctr_store_sk,
  sum(store_returns.sr_fee) AS ctr_total_return
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
order by c_customer_id
limit 100;

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
        d_year = 1998) y,
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
        d_year = 1998+1) z
 where d_week_seq1=d_week_seq2-53
 order by d_week_seq1;

--= query_3
PRAGMA AnsiImplicitCrossJoin;
select dt.d_year AS d_year,
  item.i_brand_id AS brand_id,
  item.i_brand AS brand,
  sum(store_sales.ss_sales_price) AS sum_agg
 from  date_dim dt 
      ,store_sales
      ,item
 where dt.d_date_sk = store_sales.ss_sold_date_sk
   and store_sales.ss_item_sk = item.i_item_sk
   and item.i_manufact_id = 816
   and dt.d_moy=11
 group by dt.d_year, item.i_brand, item.i_brand_id
 order by d_year, sum_agg desc, brand_id
 limit 100;

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
                 ,t_s_secyear.customer_birth_country AS customer_birth_country
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
   and t_s_firstyear.dyear =  1999
   and t_s_secyear.dyear = 1999+1
   and t_c_firstyear.dyear =  1999
   and t_c_secyear.dyear =  1999+1
   and t_w_firstyear.dyear = 1999
   and t_w_secyear.dyear = 1999+1
   and t_s_firstyear.year_total > 0
   and t_c_firstyear.year_total > 0
   and t_w_firstyear.year_total > 0
   and case when t_c_firstyear.year_total > 0 then t_c_secyear.year_total / t_c_firstyear.year_total else null end
           > case when t_s_firstyear.year_total > 0 then t_s_secyear.year_total / t_s_firstyear.year_total else null end
   and case when t_c_firstyear.year_total > 0 then t_c_secyear.year_total / t_c_firstyear.year_total else null end
           > case when t_w_firstyear.year_total > 0 then t_w_secyear.year_total / t_w_firstyear.year_total else null end
 order by t_s_secyear.customer_id
         ,t_s_secyear.customer_first_name
         ,t_s_secyear.customer_last_name
         ,t_s_secyear.customer_birth_country
limit 100;

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
       and d_date between ('2000-08-19') 
                  and '2000-09-02'
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
       and d_date between ('2000-08-19')
                  and '2000-09-02'
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
       and d_date between ('2000-08-19')
                  and '2000-09-02'
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
 
 order by channel, id
 limit 100;

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
               where d_year = 2002
 	        and d_moy = 3 )
 	and iavg.iavg_cat = i.i_category
 	and i.i_current_price > 1.2 * iavg.iavg_price
 group by a.ca_state
 having count(*) >= 10
 order by cnt, state
 limit 100;

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
       cd_gender = 'F' and 
       cd_marital_status = 'W' and
       cd_education_status = 'College' and
       (p_channel_email = 'N' or p_channel_event = 'N') and
       d_year = 2001 
 group by item.i_item_id
 order by i_item_id
 limit 100;

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
                          '47602','16704','35863','28577','83910','36201',
                          '58412','48162','28055','41419','80332',
                          '38607','77817','24891','16226','18410',
                          '21231','59345','13918','51089','20317',
                          '17167','54585','67881','78366','47770',
                          '18360','51717','73108','14440','21800',
                          '89338','45859','65501','34948','25973',
                          '73219','25333','17291','10374','18829',
                          '60736','82620','41351','52094','19326',
                          '25214','54207','40936','21814','79077',
                          '25178','75742','77454','30621','89193',
                          '27369','41232','48567','83041','71948',
                          '37119','68341','14073','16891','62878',
                          '49130','19833','24286','27700','40979',
                          '50412','81504','94835','84844','71954',
                          '39503','57649','18434','24987','12350',
                          '86379','27413','44529','98569','16515',
                          '27287','24255','21094','16005','56436',
                          '91110','68293','56455','54558','10298',
                          '83647','32754','27052','51766','19444',
                          '13869','45645','94791','57631','20712',
                          '37788','41807','46507','21727','71836',
                          '81070','50632','88086','63991','20244',
                          '31655','51782','29818','63792','68605',
                          '94898','36430','57025','20601','82080',
                          '33869','22728','35834','29086','92645',
                          '98584','98072','11652','78093','57553',
                          '43830','71144','53565','18700','90209',
                          '71256','38353','54364','28571','96560',
                          '57839','56355','50679','45266','84680',
                          '34306','34972','48530','30106','15371',
                          '92380','84247','92292','68852','13338',
                          '34594','82602','70073','98069','85066',
                          '47289','11686','98862','26217','47529',
                          '63294','51793','35926','24227','14196',
                          '24594','32489','99060','49472','43432',
                          '49211','14312','88137','47369','56877',
                          '20534','81755','15794','12318','21060',
                          '73134','41255','63073','81003','73873',
                          '66057','51184','51195','45676','92696',
                          '70450','90669','98338','25264','38919',
                          '59226','58581','60298','17895','19489',
                          '52301','80846','95464','68770','51634',
                          '19988','18367','18421','11618','67975',
                          '25494','41352','95430','15734','62585',
                          '97173','33773','10425','75675','53535',
                          '17879','41967','12197','67998','79658',
                          '59130','72592','14851','43933','68101',
                          '50636','25717','71286','24660','58058',
                          '72991','95042','15543','33122','69280',
                          '11912','59386','27642','65177','17672',
                          '33467','64592','36335','54010','18767',
                          '63193','42361','49254','33113','33159',
                          '36479','59080','11855','81963','31016',
                          '49140','29392','41836','32958','53163',
                          '13844','73146','23952','65148','93498',
                          '14530','46131','58454','13376','13378',
                          '83986','12320','17193','59852','46081',
                          '98533','52389','13086','68843','31013',
                          '13261','60560','13443','45533','83583',
                          '11489','58218','19753','22911','25115',
                          '86709','27156','32669','13123','51933',
                          '39214','41331','66943','14155','69998',
                          '49101','70070','35076','14242','73021',
                          '59494','15782','29752','37914','74686',
                          '83086','34473','15751','81084','49230',
                          '91894','60624','17819','28810','63180',
                          '56224','39459','55233','75752','43639',
                          '55349','86057','62361','50788','31830',
                          '58062','18218','85761','60083','45484',
                          '21204','90229','70041','41162','35390',
                          '16364','39500','68908','26689','52868',
                          '81335','40146','11340','61527','61794',
                          '71997','30415','59004','29450','58117',
                          '69952','33562','83833','27385','61860',
                          '96435','48333','23065','32961','84919',
                          '61997','99132','22815','56600','68730',
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
 order by s_store_name
 limit 100;

--= query_9
PRAGMA AnsiImplicitCrossJoin;
select case when (select count(*) 
                  from store_sales 
                  where ss_quantity between 1 and 20) > 1071
            then (select avg(ss_ext_tax) 
                  from store_sales 
                  where ss_quantity between 1 and 20) 
            else (select avg(ss_net_paid_inc_tax)
                  from store_sales
                  where ss_quantity between 1 and 20) end bucket1 ,
       case when (select count(*)
                  from store_sales
                  where ss_quantity between 21 and 40) > 39161
            then (select avg(ss_ext_tax)
                  from store_sales
                  where ss_quantity between 21 and 40) 
            else (select avg(ss_net_paid_inc_tax)
                  from store_sales
                  where ss_quantity between 21 and 40) end bucket2,
       case when (select count(*)
                  from store_sales
                  where ss_quantity between 41 and 60) > 29434
            then (select avg(ss_ext_tax)
                  from store_sales
                  where ss_quantity between 41 and 60)
            else (select avg(ss_net_paid_inc_tax)
                  from store_sales
                  where ss_quantity between 41 and 60) end bucket3,
       case when (select count(*)
                  from store_sales
                  where ss_quantity between 61 and 80) > 6568
            then (select avg(ss_ext_tax)
                  from store_sales
                  where ss_quantity between 61 and 80)
            else (select avg(ss_net_paid_inc_tax)
                  from store_sales
                  where ss_quantity between 61 and 80) end bucket4,
       case when (select count(*)
                  from store_sales
                  where ss_quantity between 81 and 100) > 21216
            then (select avg(ss_ext_tax)
                  from store_sales
                  where ss_quantity between 81 and 100)
            else (select avg(ss_net_paid_inc_tax)
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
  ca_county in ('Fairfield County','Campbell County','Washtenaw County','Escambia County','Cleburne County') and
  cd_demo_sk = c.c_current_cdemo_sk and 
  c.c_customer_sk in (select ss_customer_sk from store_sales,date_dim where ss_sold_date_sk = d_date_sk and
                d_year = 2001 and
                d_moy between 3 and 3+3) and
   (c.c_customer_sk in (select ws_bill_customer_sk from web_sales,date_dim where ws_sold_date_sk = d_date_sk and
                  d_year = 2001 and
                  d_moy between 3 ANd 3+3) or 
    c.c_customer_sk in (select cs_ship_customer_sk from catalog_sales,date_dim where cs_sold_date_sk = d_date_sk and
                  d_year = 2001 and
                  d_moy between 3 and 3+3))
 group by customer_demographics.cd_gender, customer_demographics.cd_marital_status, customer_demographics.cd_education_status, customer_demographics.cd_purchase_estimate, customer_demographics.cd_credit_rating, customer_demographics.cd_dep_count, customer_demographics.cd_dep_employed_count, customer_demographics.cd_dep_college_count
 order by cd_gender, cd_marital_status, cd_education_status, cd_purchase_estimate, cd_credit_rating, cd_dep_count, cd_dep_employed_count, cd_dep_college_count
 limit 100;

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
                 ,t_s_secyear.customer_email_address AS customer_email_address
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
         and t_s_firstyear.dyear = 1998
         and t_s_secyear.dyear = 1998+1
         and t_w_firstyear.dyear = 1998
         and t_w_secyear.dyear = 1998+1
         and t_s_firstyear.year_total > 0
         and t_w_firstyear.year_total > 0
         and case when t_w_firstyear.year_total > 0 then t_w_secyear.year_total / t_w_firstyear.year_total else 0.0 end
             > case when t_s_firstyear.year_total > 0 then t_s_secyear.year_total / t_s_firstyear.year_total else 0.0 end
 order by t_s_secyear.customer_id
         ,t_s_secyear.customer_first_name
         ,t_s_secyear.customer_last_name
         ,t_s_secyear.customer_email_address
limit 100;

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
  	and i_category in ('Men', 'Books', 'Electronics')
  	and ws_sold_date_sk = d_date_sk
	and d_date between ('2001-06-15') 
				and '2001-07-15'
group by item.i_item_id, item.i_item_desc, item.i_category, item.i_class, item.i_current_price
 order by i_category, i_class, i_item_id, i_item_desc, revenueratio
 limit 100;

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
  and cd_education_status = 'College'
  and ss_sales_price between 100.00 and 150.00
  and hd_dep_count = 3   
     )or
     (ss_hdemo_sk=hd_demo_sk
  and cd_demo_sk = ss_cdemo_sk
  and cd_marital_status = 'D'
  and cd_education_status = 'Primary'
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
  and ca_state in ('IL', 'TN', 'TX')
  and ss_net_profit between 100 and 200  
     ) or
     (ss_addr_sk = ca_address_sk
  and ca_country = 'United States'
  and ca_state in ('WY', 'OH', 'ID')
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
 
 order by channel, i_brand_id, i_class_id, i_category_id
 limit 100;

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
                       and d_dom = 3)
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
                       and d_dom = 3)
 group by item.i_brand_id, item.i_class_id, item.i_category_id
 having sum(store_sales.ss_quantity*store_sales.ss_list_price) > (select average_sales from $avg_sales AS avg_sales)) last_year
 where this_year.i_brand_id= last_year.i_brand_id
   and this_year.i_class_id = last_year.i_class_id
   and this_year.i_category_id = last_year.i_category_id
 order by this_year.channel, this_year.i_brand_id, this_year.i_class_id, this_year.i_category_id
 limit 100;

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
 order by ca_zip
 limit 100;

--= query_16
PRAGMA AnsiImplicitCrossJoin;
select count(distinct cs1.cs_order_number) AS `order count`,
  sum(cs1.cs_ext_ship_cost) AS `total shipping cost`,
  sum(cs1.cs_net_profit) AS `total net profit`
 from catalog_sales cs1, date_dim, customer_address, call_center
where d_date between '2002-04-01' and '2002-05-31'
  and cs1.cs_ship_date_sk = d_date_sk
  and cs1.cs_ship_addr_sk = ca_address_sk
  and ca_state = 'PA'
  and cs1.cs_call_center_sk = cc_call_center_sk
  and cc_county in ('Williamson County','Williamson County','Williamson County','Williamson County','Williamson County')
  and cs1.cs_order_number in (select cs_order_number AS cs_order_number
 from catalog_sales group by cs_order_number
 having count(distinct cs_warehouse_sk) > 1)
  and cs1.cs_order_number not in (select cr_order_number from catalog_returns)
order by `order count`
 limit 100;

--= query_17
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  store.s_state AS s_state,
  count(store_sales.ss_quantity) AS store_sales_quantitycount,
  avg(store_sales.ss_quantity) AS store_sales_quantityave,
  stddev_samp(store_sales.ss_quantity) AS store_sales_quantitystdev,
  stddev_samp(store_sales.ss_quantity)/avg(store_sales.ss_quantity) AS store_sales_quantitycov,
  count(store_returns.sr_return_quantity) AS store_returns_quantitycount,
  avg(store_returns.sr_return_quantity) AS store_returns_quantityave,
  stddev_samp(store_returns.sr_return_quantity) AS store_returns_quantitystdev,
  stddev_samp(store_returns.sr_return_quantity)/avg(store_returns.sr_return_quantity) AS store_returns_quantitycov,
  count(catalog_sales.cs_quantity) AS catalog_sales_quantitycount,
  avg(catalog_sales.cs_quantity) AS catalog_sales_quantityave,
  stddev_samp(catalog_sales.cs_quantity) AS catalog_sales_quantitystdev,
  stddev_samp(catalog_sales.cs_quantity)/avg(catalog_sales.cs_quantity) AS catalog_sales_quantitycov
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
 order by i_item_id, i_item_desc, s_state
 limit 100;

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
       cd1.cd_education_status = 'Primary' and
       c_current_cdemo_sk = cd2.cd_demo_sk and
       c_current_addr_sk = ca_address_sk and
       c_birth_month in (1,3,7,11,10,4) and
       d_year = 2001 and
       ca_state in ('AL','MO','TN'
                   ,'GA','MT','IN','CA')
 group by  rollup (item.i_item_id, customer_address.ca_country, customer_address.ca_state, customer_address.ca_county)
 
 order by ca_country, ca_state, ca_county, i_item_id
 limit 100;

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
   and i_manager_id=14
   and d_moy=11
   and d_year=2002
   and ss_customer_sk = c_customer_sk 
   and c_current_addr_sk = ca_address_sk
   and Unicode::Substring(ca_zip, CAST(0 AS Uint32), CAST(5 AS Uint32)) <> Unicode::Substring(s_zip, CAST(0 AS Uint32), CAST(5 AS Uint32)) 
   and ss_store_sk = s_store_sk 
 group by item.i_brand, item.i_brand_id, item.i_manufact_id, item.i_manufact
 order by ext_price desc, brand, brand_id, i_manufact_id, i_manufact
 limit 100;

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
 from	catalog_sales
     ,item 
     ,date_dim
 where cs_item_sk = i_item_sk 
   and i_category in ('Books', 'Music', 'Sports')
   and cs_sold_date_sk = d_date_sk
 and d_date between ('2002-06-18') 
 				and '2002-07-18'
 group by item.i_item_id, item.i_item_desc, item.i_category, item.i_class, item.i_current_price
 order by i_category, i_class, i_item_id, i_item_desc, revenueratio
 limit 100;

--= query_21
PRAGMA AnsiImplicitCrossJoin;
select  *
 from(select warehouse.w_warehouse_name AS w_warehouse_name,
  item.i_item_id AS i_item_id,
  sum(case when ((date_dim.d_date) < ('1999-06-22'))
	                then inventory.inv_quantity_on_hand 
                      else 0 end) AS inv_before,
  sum(case when ((date_dim.d_date) >= ('1999-06-22'))
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
     and d_date between '1999-05-23'
                    and '1999-07-22'
   group by warehouse.w_warehouse_name, item.i_item_id
 ) x
 where (case when inv_before > 0 
             then inv_after / inv_before 
             else null
             end) between 2.0/3.0 and 3.0/2.0
 order by w_warehouse_name
         ,i_item_id
 limit 100;

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

 order by qoh, i_product_name, i_brand, i_class, i_category
 limit 100;

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
 having sum(store_sales.ss_quantity*store_sales.ss_sales_price) > (95/100.0) * (select
  *
from
 $max_store_sales AS max_store_sales));

  select  sum(sales)
 from (select cs_quantity*cs_list_price sales
       from catalog_sales
           ,date_dim 
       where d_year = 2000 
         and d_moy = 7 
         and cs_sold_date_sk = d_date_sk 
         and cs_item_sk in (select item_sk from $frequent_ss_items AS frequent_ss_items)
         and cs_bill_customer_sk in (select c_customer_sk from $best_ss_customer AS best_ss_customer)
      union all
      select ws_quantity*ws_list_price sales
       from web_sales 
           ,date_dim 
       where d_year = 2000 
         and d_moy = 7 
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
 having sum(store_sales.ss_quantity*store_sales.ss_sales_price) > (95/100.0) * (select
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
         and d_moy = 7 
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
         and d_moy = 7 
         and ws_sold_date_sk = d_date_sk 
         and ws_item_sk in (select item_sk from $frequent_ss_items AS frequent_ss_items)
         and ws_bill_customer_sk in (select c_customer_sk from $best_ss_customer AS best_ss_customer)
         and ws_bill_customer_sk = c_customer_sk
       group by customer.c_last_name, customer.c_first_name
 ) AS _sq2 
     order by c_last_name,c_first_name,sales
  limit 100;

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
and s_market_id=5
group by customer.c_last_name, customer.c_first_name, store.s_store_name, customer_address.ca_state, store.s_state, item.i_color, item.i_current_price, item.i_manager_id, item.i_units, item.i_size
 );

select c_last_name AS c_last_name,
  c_first_name AS c_first_name,
  s_store_name AS s_store_name,
  sum(netpaid) AS paid
 from $ssales AS ssales
where i_color = 'aquamarine'
group by c_last_name, c_first_name, s_store_name
 having sum(netpaid) > (select 0.05*avg(netpaid)
                                 from $ssales AS ssales)
order by c_last_name, c_first_name, s_store_name;

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
  and s_market_id = 5
group by customer.c_last_name, customer.c_first_name, store.s_store_name, customer_address.ca_state, store.s_state, item.i_color, item.i_current_price, item.i_manager_id, item.i_units, item.i_size
 );

select c_last_name AS c_last_name,
  c_first_name AS c_first_name,
  s_store_name AS s_store_name,
  sum(netpaid) AS paid
 from $ssales AS ssales
where i_color = 'seashell'
group by c_last_name, c_first_name, s_store_name
 having sum(netpaid) > (select 0.05*avg(netpaid)
                           from $ssales AS ssales)
order by c_last_name, c_first_name, s_store_name;

--= query_25
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  store.s_store_id AS s_store_id,
  store.s_store_name AS s_store_name,
  max(store_sales.ss_net_profit) AS store_sales_profit,
  max(store_returns.sr_net_loss) AS store_returns_loss,
  max(catalog_sales.cs_net_profit) AS catalog_sales_profit
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
 and d1.d_year = 1999
 and d1.d_date_sk = ss_sold_date_sk
 and i_item_sk = ss_item_sk
 and s_store_sk = ss_store_sk
 and ss_customer_sk = sr_customer_sk
 and ss_item_sk = sr_item_sk
 and ss_ticket_number = sr_ticket_number
 and sr_returned_date_sk = d2.d_date_sk
 and d2.d_moy               between 4 and  10
 and d2.d_year              = 1999
 and sr_customer_sk = cs_bill_customer_sk
 and sr_item_sk = cs_item_sk
 and cs_sold_date_sk = d3.d_date_sk
 and d3.d_moy               between 4 and  10 
 and d3.d_year              = 1999
 group by item.i_item_id, item.i_item_desc, store.s_store_id, store.s_store_name
 order by i_item_id, i_item_desc, s_store_id, s_store_name
 limit 100;

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
       cd_marital_status = 'W' and
       cd_education_status = 'Unknown' and
       (p_channel_email = 'N' or p_channel_event = 'N') and
       d_year = 2002 
 group by item.i_item_id
 order by i_item_id
 limit 100;

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
       cd_marital_status = 'W' and
       cd_education_status = 'Secondary' and
       d_year = 1999 and
       s_state in ('TN','TN', 'TN', 'TN', 'TN', 'TN')
 group by  rollup (item.i_item_id, store.s_state)
 
 order by i_item_id, s_state
 limit 100;

--= query_28
PRAGMA AnsiImplicitCrossJoin;
select  *
from (select avg(ss_list_price) B1_LP
            ,count(ss_list_price) B1_CNT
            ,count(distinct ss_list_price) B1_CNTD
      from store_sales
      where ss_quantity between 0 and 5
        and (ss_list_price between 107 and 107+10 
             or ss_coupon_amt between 1319 and 1319+1000
             or ss_wholesale_cost between 60 and 60+20)) B1,
     (select avg(ss_list_price) B2_LP
            ,count(ss_list_price) B2_CNT
            ,count(distinct ss_list_price) B2_CNTD
      from store_sales
      where ss_quantity between 6 and 10
        and (ss_list_price between 23 and 23+10
          or ss_coupon_amt between 825 and 825+1000
          or ss_wholesale_cost between 43 and 43+20)) B2,
     (select avg(ss_list_price) B3_LP
            ,count(ss_list_price) B3_CNT
            ,count(distinct ss_list_price) B3_CNTD
      from store_sales
      where ss_quantity between 11 and 15
        and (ss_list_price between 74 and 74+10
          or ss_coupon_amt between 4381 and 4381+1000
          or ss_wholesale_cost between 57 and 57+20)) B3,
     (select avg(ss_list_price) B4_LP
            ,count(ss_list_price) B4_CNT
            ,count(distinct ss_list_price) B4_CNTD
      from store_sales
      where ss_quantity between 16 and 20
        and (ss_list_price between 89 and 89+10
          or ss_coupon_amt between 3117 and 3117+1000
          or ss_wholesale_cost between 68 and 68+20)) B4,
     (select avg(ss_list_price) B5_LP
            ,count(ss_list_price) B5_CNT
            ,count(distinct ss_list_price) B5_CNTD
      from store_sales
      where ss_quantity between 21 and 25
        and (ss_list_price between 58 and 58+10
          or ss_coupon_amt between 9402 and 9402+1000
          or ss_wholesale_cost between 38 and 38+20)) B5,
     (select avg(ss_list_price) B6_LP
            ,count(ss_list_price) B6_CNT
            ,count(distinct ss_list_price) B6_CNTD
      from store_sales
      where ss_quantity between 26 and 30
        and (ss_list_price between 64 and 64+10
          or ss_coupon_amt between 5792 and 5792+1000
          or ss_wholesale_cost between 73 and 73+20)) B6
limit 100;

--= query_29
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  store.s_store_id AS s_store_id,
  store.s_store_name AS s_store_name,
  max(store_sales.ss_quantity) AS store_sales_quantity,
  max(store_returns.sr_return_quantity) AS store_returns_quantity,
  max(catalog_sales.cs_quantity) AS catalog_sales_quantity
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
     d1.d_moy               = 4 
 and d1.d_year              = 1998
 and d1.d_date_sk           = ss_sold_date_sk
 and i_item_sk              = ss_item_sk
 and s_store_sk             = ss_store_sk
 and ss_customer_sk         = sr_customer_sk
 and ss_item_sk             = sr_item_sk
 and ss_ticket_number       = sr_ticket_number
 and sr_returned_date_sk    = d2.d_date_sk
 and d2.d_moy               between 4 and  4 + 3 
 and d2.d_year              = 1998
 and sr_customer_sk         = cs_bill_customer_sk
 and sr_item_sk             = cs_item_sk
 and cs_sold_date_sk        = d3.d_date_sk     
 and d3.d_year              in (1998,1998+1,1998+2)
 group by item.i_item_id, item.i_item_desc, store.s_store_id, store.s_store_name
 order by i_item_id, i_item_desc, s_store_id, s_store_name
 limit 100;

--= query_30
PRAGMA AnsiImplicitCrossJoin;
$customer_total_return = (select web_returns.wr_returning_customer_sk AS ctr_customer_sk,
  customer_address.ca_state AS ctr_state,
  sum(web_returns.wr_return_amt) AS ctr_total_return
 from web_returns
     ,date_dim
     ,customer_address
 where wr_returned_date_sk = d_date_sk 
   and d_year =2000
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
       and ca_state = 'AR'
       and ctr1.ctr_customer_sk = c_customer_sk
 order by c_customer_id,c_salutation,c_first_name,c_last_name,c_preferred_cust_flag
                  ,c_birth_day,c_birth_month,c_birth_year,c_birth_country,c_login,c_email_address
                  ,c_last_review_date,ctr_total_return
limit 100;

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
    and ss1.d_year = 1999
    and ss1.ca_county = ss2.ca_county
    and ss2.d_qoy = 2
    and ss2.d_year = 1999
 and ss2.ca_county = ss3.ca_county
    and ss3.d_qoy = 3
    and ss3.d_year = 1999
    and ss1.ca_county = ws1.ca_county
    and ws1.d_qoy = 1
    and ws1.d_year = 1999
    and ws1.ca_county = ws2.ca_county
    and ws2.d_qoy = 2
    and ws2.d_year = 1999
    and ws1.ca_county = ws3.ca_county
    and ws3.d_qoy = 3
    and ws3.d_year =1999
    and case when ws1.web_sales > 0 then ws2.web_sales/ws1.web_sales else null end 
       > case when ss1.store_sales > 0 then ss2.store_sales/ss1.store_sales else null end
    and case when ws2.web_sales > 0 then ws3.web_sales/ws2.web_sales else null end
       > case when ss2.store_sales > 0 then ss3.store_sales/ss2.store_sales else null end
 order by store_q2_q3_increase;

--= query_32
PRAGMA AnsiImplicitCrossJoin;
$disc = (select catalog_sales.cs_item_sk AS ditem,
  1.3 * avg(catalog_sales.cs_ext_discount_amt) AS avg_disc
 from catalog_sales, date_dim
  where d_date between '2001-03-09' and '2001-06-07'
    and d_date_sk = cs_sold_date_sk
  group by catalog_sales.cs_item_sk
 );

select sum(catalog_sales.cs_ext_discount_amt) AS `excess discount amount`
 from catalog_sales, item, date_dim, $disc AS disc
where i_manufact_id = 722
  and i_item_sk = cs_item_sk
  and d_date between '2001-03-09' and '2001-06-07'
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
where i_category in ('Books'))
 and     ss_item_sk              = i_item_sk
 and     ss_sold_date_sk         = d_date_sk
 and     d_year                  = 2001
 and     d_moy                   = 3
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
where i_category in ('Books'))
 and     cs_item_sk              = i_item_sk
 and     cs_sold_date_sk         = d_date_sk
 and     d_year                  = 2001
 and     d_moy                   = 3
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
where i_category in ('Books'))
 and     ws_item_sk              = i_item_sk
 and     ws_sold_date_sk         = d_date_sk
 and     d_year                  = 2001
 and     d_moy                   = 3
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
 order by total_sales
 limit 100;

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
    and (household_demographics.hd_buy_potential = '1001-5000' or
         household_demographics.hd_buy_potential = '0-500')
    and household_demographics.hd_vehicle_count > 0
    and (case when household_demographics.hd_vehicle_count > 0 
	then household_demographics.hd_dep_count/ household_demographics.hd_vehicle_count 
	else null 
	end)  > 1.2
    and date_dim.d_year in (2000,2000+1,2000+2)
    and store.s_county in ('Williamson County','Williamson County','Williamson County','Williamson County',
                           'Williamson County','Williamson County','Williamson County','Williamson County')
    group by store_sales.ss_ticket_number, store_sales.ss_customer_sk
 ) dn,customer
    where ss_customer_sk = c_customer_sk
      and cnt between 15 and 20
    order by c_last_name,c_first_name,c_salutation,c_preferred_cust_flag desc, ss_ticket_number;

--= query_35
PRAGMA AnsiImplicitCrossJoin;
select ca.ca_state AS ca_state,
  customer_demographics.cd_gender AS cd_gender,
  customer_demographics.cd_marital_status AS cd_marital_status,
  customer_demographics.cd_dep_count AS cd_dep_count,
  count(*) AS cnt1,
  avg(customer_demographics.cd_dep_count) AS aggone1,
  stddev_samp(customer_demographics.cd_dep_count) AS aggtwo1,
  sum(customer_demographics.cd_dep_count) AS aggthree1,
  customer_demographics.cd_dep_employed_count AS cd_dep_employed_count,
  count(*) AS cnt2,
  avg(customer_demographics.cd_dep_employed_count) AS aggone2,
  stddev_samp(customer_demographics.cd_dep_employed_count) AS aggtwo2,
  sum(customer_demographics.cd_dep_employed_count) AS aggthree2,
  customer_demographics.cd_dep_college_count AS cd_dep_college_count,
  count(*) AS cnt3,
  avg(customer_demographics.cd_dep_college_count) AS aggone3,
  stddev_samp(customer_demographics.cd_dep_college_count) AS aggtwo3,
  sum(customer_demographics.cd_dep_college_count) AS aggthree3
 from
  customer c,customer_address ca,customer_demographics
 where
  c.c_current_addr_sk = ca.ca_address_sk and
  cd_demo_sk = c.c_current_cdemo_sk and 
  c.c_customer_sk in (select ss_customer_sk from store_sales,date_dim where ss_sold_date_sk = d_date_sk and
                d_year = 1999 and
                d_qoy < 4) and
   (c.c_customer_sk in (select ws_bill_customer_sk from web_sales,date_dim where ws_sold_date_sk = d_date_sk and
                  d_year = 1999 and
                  d_qoy < 4) or 
    c.c_customer_sk in (select cs_ship_customer_sk from catalog_sales,date_dim where cs_sold_date_sk = d_date_sk and
                  d_year = 1999 and
                  d_qoy < 4))
 group by ca.ca_state, customer_demographics.cd_gender, customer_demographics.cd_marital_status, customer_demographics.cd_dep_count, customer_demographics.cd_dep_employed_count, customer_demographics.cd_dep_college_count
 order by ca_state, cd_gender, cd_marital_status, cd_dep_count, cd_dep_employed_count, cd_dep_college_count
 limit 100;

--= query_36
PRAGMA AnsiImplicitCrossJoin;
select sum(store_sales.ss_net_profit)/sum(store_sales.ss_ext_sales_price) AS gross_margin,
  item.i_category AS i_category,
  item.i_class AS i_class,
  grouping(item.i_category)+grouping(item.i_class) AS lochierarchy,
  rank() over (
 	partition by grouping(item.i_category)+grouping(item.i_class),
 	case when grouping(item.i_class) = 0 then item.i_category  else null end 
 	order by sum(store_sales.ss_net_profit)/sum(store_sales.ss_ext_sales_price) asc) AS rank_within_parent
 from
    store_sales
   ,date_dim       d1
   ,item
   ,store
 where
    d1.d_year = 2000 
 and d1.d_date_sk = ss_sold_date_sk
 and i_item_sk  = ss_item_sk 
 and s_store_sk  = ss_store_sk
 and s_state in ('TN','TN','TN','TN',
                 'TN','TN','TN','TN')
 group by  rollup(item.i_category,item.i_class)
 
 order by lochierarchy desc, case when lochierarchy = 0 then i_category  else null end, rank_within_parent
 limit 100;

--= query_37
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  item.i_current_price AS i_current_price
 from item, inventory, date_dim, catalog_sales
 where i_current_price between 29 and 29 + 30
 and inv_item_sk = i_item_sk
 and d_date_sk=inv_date_sk
 and d_date between ('2002-03-29') and '2002-05-28'
 and i_manufact_id in (705,742,777,944)
 and inv_quantity_on_hand between 100 and 500
 and cs_item_sk = i_item_sk
 group by item.i_item_id, item.i_item_desc, item.i_current_price
 order by i_item_id
 limit 100;

--= query_38
PRAGMA AnsiImplicitCrossJoin;
select  count(*) from (
    select distinct c_last_name, c_first_name, d_date
    from store_sales, date_dim, customer
          where store_sales.ss_sold_date_sk = date_dim.d_date_sk
      and store_sales.ss_customer_sk = customer.c_customer_sk
      and d_month_seq between 1189 and 1189 + 11
  intersect
    select distinct c_last_name, c_first_name, d_date
    from catalog_sales, date_dim, customer
          where catalog_sales.cs_sold_date_sk = date_dim.d_date_sk
      and catalog_sales.cs_bill_customer_sk = customer.c_customer_sk
      and d_month_seq between 1189 and 1189 + 11
  intersect
    select distinct c_last_name, c_first_name, d_date
    from web_sales, date_dim, customer
          where web_sales.ws_sold_date_sk = date_dim.d_date_sk
      and web_sales.ws_bill_customer_sk = customer.c_customer_sk
      and d_month_seq between 1189 and 1189 + 11
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
  stddev_samp(inventory.inv_quantity_on_hand) AS stdev,
  avg(inventory.inv_quantity_on_hand) AS mean
 from inventory
          ,item
          ,warehouse
          ,date_dim
      where inv_item_sk = i_item_sk
        and inv_warehouse_sk = w_warehouse_sk
        and inv_date_sk = d_date_sk
        and d_year =2000
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
order by inv1.w_warehouse_sk,inv1.i_item_sk,inv1.d_moy,inv1.mean,inv1.cov
        ,inv2.d_moy,inv2.mean, inv2.cov;

--= query_39_b
PRAGMA AnsiImplicitCrossJoin;
$inv = (select w_warehouse_name,w_warehouse_sk,i_item_sk,d_moy
       ,stdev,mean, case mean when 0 then null else stdev/mean end cov
 from(select warehouse.w_warehouse_name AS w_warehouse_name,
  warehouse.w_warehouse_sk AS w_warehouse_sk,
  item.i_item_sk AS i_item_sk,
  date_dim.d_moy AS d_moy,
  stddev_samp(inventory.inv_quantity_on_hand) AS stdev,
  avg(inventory.inv_quantity_on_hand) AS mean
 from inventory
          ,item
          ,warehouse
          ,date_dim
      where inv_item_sk = i_item_sk
        and inv_warehouse_sk = w_warehouse_sk
        and inv_date_sk = d_date_sk
        and d_year =2000
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
order by inv1.w_warehouse_sk,inv1.i_item_sk,inv1.d_moy,inv1.mean,inv1.cov
        ,inv2.d_moy,inv2.mean, inv2.cov;

--= query_40
PRAGMA AnsiImplicitCrossJoin;
select warehouse.w_state AS w_state,
  item.i_item_id AS i_item_id,
  sum(case when ((date_dim.d_date) < ('2001-05-02')) 
 		then catalog_sales.cs_sales_price - coalesce(catalog_returns.cr_refunded_cash,0) else 0 end) AS sales_before,
  sum(case when ((date_dim.d_date) >= ('2001-05-02')) 
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
 and d_date between '2001-04-02'
                and '2001-06-01' 
 group by warehouse.w_state, item.i_item_id
 order by w_state, i_item_id
 limit 100;

--= query_41
PRAGMA AnsiImplicitCrossJoin;
$mfg = (select distinct i_manufact from item
  where (((i_category = 'Women' and (i_color = 'forest' or i_color = 'lime') and (i_units = 'Pallet' or i_units = 'Pound') and (i_size = 'economy' or i_size = 'small'))
       or (i_category = 'Women' and (i_color = 'navy' or i_color = 'slate') and (i_units = 'Gross' or i_units = 'Bunch') and (i_size = 'extra large' or i_size = 'petite'))
       or (i_category = 'Men' and (i_color = 'powder' or i_color = 'sky') and (i_units = 'Dozen' or i_units = 'Lb') and (i_size = 'N/A' or i_size = 'large'))
       or (i_category = 'Men' and (i_color = 'maroon' or i_color = 'smoke') and (i_units = 'Ounce' or i_units = 'Case') and (i_size = 'economy' or i_size = 'small')))
     or ((i_category = 'Women' and (i_color = 'dark' or i_color = 'aquamarine') and (i_units = 'Ton' or i_units = 'Tbl') and (i_size = 'economy' or i_size = 'small'))
       or (i_category = 'Women' and (i_color = 'frosted' or i_color = 'plum') and (i_units = 'Dram' or i_units = 'Box') and (i_size = 'extra large' or i_size = 'petite'))
       or (i_category = 'Men' and (i_color = 'papaya' or i_color = 'peach') and (i_units = 'Bundle' or i_units = 'Carton') and (i_size = 'N/A' or i_size = 'large'))
       or (i_category = 'Men' and (i_color = 'firebrick' or i_color = 'sienna') and (i_units = 'Cup' or i_units = 'Each') and (i_size = 'economy' or i_size = 'small')))));

select distinct(i_product_name)
 from item i1
 where i_manufact_id between 704 and 704+40
   and i1.i_manufact in (select i_manufact from $mfg AS mfg)
 order by i_product_name
 limit 100;

--= query_42
PRAGMA AnsiImplicitCrossJoin;
select dt.d_year AS d_year,
  item.i_category_id AS i_category_id,
  item.i_category AS i_category,
  sum(store_sales.ss_ext_sales_price) AS c9
 from 	date_dim dt
 	,store_sales
 	,item
 where dt.d_date_sk = store_sales.ss_sold_date_sk
 	and store_sales.ss_item_sk = item.i_item_sk
 	and item.i_manager_id = 1  	
 	and dt.d_moy=11
 	and dt.d_year=1998
 group by dt.d_year, item.i_category_id, item.i_category
 order by c9 desc, d_year, i_category_id, i_category
 limit 100;

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
 order by s_store_name, s_store_id, sun_sales, mon_sales, tue_sales, wed_sales, thu_sales, fri_sales, sat_sales
 limit 100;

--= query_44
PRAGMA AnsiImplicitCrossJoin;
select  asceding.rnk AS rnk, i1.i_product_name best_performing, i2.i_product_name worst_performing
from(select *
     from (select item_sk,rank() over (order by rank_col asc) rnk
           from (select ss_item_sk AS item_sk,
  avg(ss_net_profit) AS rank_col
 from store_sales ss1
                 where ss_store_sk = 4
                 group by ss_item_sk
 having avg(ss_net_profit) > 0.9*(select avg(ss_net_profit) AS rank_col
 from store_sales
                                                  where ss_store_sk = 4
                                                    and ss_hdemo_sk is null
                                                  group by ss_store_sk
 ))V1)V11
     where rnk  < 11) asceding,
    (select *
     from (select item_sk,rank() over (order by rank_col desc) rnk
           from (select ss_item_sk AS item_sk,
  avg(ss_net_profit) AS rank_col
 from store_sales ss1
                 where ss_store_sk = 4
                 group by ss_item_sk
 having avg(ss_net_profit) > 0.9*(select avg(ss_net_profit) AS rank_col
 from store_sales
                                                  where ss_store_sk = 4
                                                    and ss_hdemo_sk is null
                                                  group by ss_store_sk
 ))V2)V21
     where rnk  < 11) descending,
item i1,
item i2
where asceding.rnk = descending.rnk 
  and i1.i_item_sk=asceding.item_sk
  and i2.i_item_sk=descending.item_sk
order by asceding.rnk
limit 100;

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
 	and d_qoy = 1 and d_year = 2000
 group by customer_address.ca_zip, customer_address.ca_city
 order by ca_zip, ca_city
 limit 100;

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
    and (household_demographics.hd_dep_count = 8 or
         household_demographics.hd_vehicle_count= 0)
    and date_dim.d_dow in (6,0)
    and date_dim.d_year in (2000,2000+1,2000+2) 
    and store.s_city in ('Midway','Fairview','Fairview','Midway','Fairview') 
    group by store_sales.ss_ticket_number, store_sales.ss_customer_sk, store_sales.ss_addr_sk, customer_address.ca_city
 ) dn,customer,customer_address current_addr
    where ss_customer_sk = c_customer_sk
      and customer.c_current_addr_sk = current_addr.ca_address_sk
      and current_addr.ca_city <> bought_city
  order by c_last_name
          ,c_first_name
          ,ca_city
          ,bought_city
          ,ss_ticket_number
  limit 100;

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
           order by date_dim.d_year, date_dim.d_moy) AS rn
 from item, store_sales, date_dim, store
 where ss_item_sk = i_item_sk and
       ss_sold_date_sk = d_date_sk and
       ss_store_sk = s_store_sk and
       (
         d_year = 2000 or
         ( d_year = 2000-1 and d_moy =12) or
         ( d_year = 2000+1 and d_moy =1)
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

  select  *
 from $v2 AS v2
 where  d_year = 2000 and    
        avg_monthly_sales > 0 and
        case when avg_monthly_sales > 0 then abs(sum_sales - avg_monthly_sales) / avg_monthly_sales else null end > 0.1
 order by sum_sales - avg_monthly_sales, nsum
 limit 100;

--= query_48
PRAGMA AnsiImplicitCrossJoin;
select sum (store_sales.ss_quantity) AS c11
 from store_sales, store, customer_demographics, customer_address, date_dim
 where s_store_sk = ss_store_sk
 and  ss_sold_date_sk = d_date_sk and d_year = 2001
 and  
 (
  (
   cd_demo_sk = ss_cdemo_sk
   and 
   cd_marital_status = 'S'
   and 
   cd_education_status = 'Secondary'
   and 
   ss_sales_price between 100.00 and 150.00  
   )
 or
  (
  cd_demo_sk = ss_cdemo_sk
   and 
   cd_marital_status = 'M'
   and 
   cd_education_status = '2 yr Degree'
   and 
   ss_sales_price between 50.00 and 100.00   
  )
 or 
 (
  cd_demo_sk = ss_cdemo_sk
  and 
   cd_marital_status = 'D'
   and 
   cd_education_status = 'Advanced Degree'
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
  ca_state in ('ND', 'NY', 'SD')
  and ss_net_profit between 0 and 2000  
  )
 or
  (ss_addr_sk = ca_address_sk
  and
  ca_country = 'United States'
  and
  ca_state in ('MD', 'GA', 'KS')
  and ss_net_profit between 150 and 3000 
  )
 or
  (ss_addr_sk = ca_address_sk
  and
  ca_country = 'United States'
  and
  ca_state in ('CO', 'MN', 'NC')
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
 	,rank() over (order by return_ratio) as return_rank
 	,rank() over (order by currency_ratio) as currency_rank
 	from
 	(	select ws.ws_item_sk AS item,
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
                         and d_year = 1998
                         and d_moy = 11
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
 	,rank() over (order by return_ratio) as return_rank
 	,rank() over (order by currency_ratio) as currency_rank
 	from
 	(	select cs.cs_item_sk AS item,
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
                         and d_year = 1998
                         and d_moy = 11
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
 	,rank() over (order by return_ratio) as return_rank
 	,rank() over (order by currency_ratio) as currency_rank
 	from
 	(	select sts.ss_item_sk AS item,
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
                         and d_year = 1998
                         and d_moy = 11
 		group by sts.ss_item_sk
 ) in_store
 ) store
 where  (
 store.return_rank <= 10
 or 
 store.currency_rank <= 10
 )
 ) AS _sq1
 order by channel, return_rank, currency_rank, item
 limit 100;

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
 order by s_store_name, s_company_id, s_street_number, s_street_name, s_street_type, s_suite_number, s_city, s_county, s_state, s_zip
 limit 100;

--= query_51
PRAGMA AnsiImplicitCrossJoin;
$web_v1 = (select web_sales.ws_item_sk AS item_sk,
  date_dim.d_date AS d_date,
  sum(sum(web_sales.ws_sales_price))
      over (partition by web_sales.ws_item_sk order by date_dim.d_date rows between unbounded preceding and current row) AS cume_sales
 from web_sales
    ,date_dim
where ws_sold_date_sk=d_date_sk
  and d_month_seq between 1212 and 1212+11
  and ws_item_sk is not NULL
group by web_sales.ws_item_sk, date_dim.d_date
 );
$store_v1 = (select store_sales.ss_item_sk AS item_sk,
  date_dim.d_date AS d_date,
  sum(sum(store_sales.ss_sales_price))
      over (partition by store_sales.ss_item_sk order by date_dim.d_date rows between unbounded preceding and current row) AS cume_sales
 from store_sales
    ,date_dim
where ss_sold_date_sk=d_date_sk
  and d_month_seq between 1212 and 1212+11
  and ss_item_sk is not NULL
group by store_sales.ss_item_sk, date_dim.d_date
 );

 select  *
from (select item_sk
     ,d_date
     ,web_sales
     ,store_sales
     ,max(web_sales)
         over (partition by item_sk order by d_date rows between unbounded preceding and current row) web_cumulative
     ,max(store_sales)
         over (partition by item_sk order by d_date rows between unbounded preceding and current row) store_cumulative
     from (select case when web.item_sk is not null then web.item_sk else store.item_sk end item_sk
                 ,case when web.d_date is not null then web.d_date else store.d_date end d_date
                 ,web.cume_sales web_sales
                 ,store.cume_sales store_sales
           from $web_v1 AS web full outer join $store_v1 AS store on (web.item_sk = store.item_sk
                                                          and web.d_date = store.d_date)
          )x )y
where web_cumulative > store_cumulative
order by item_sk
        ,d_date
limit 100;

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
    and dt.d_moy=12
    and dt.d_year=2000
 group by dt.d_year, item.i_brand, item.i_brand_id
 order by d_year, ext_price desc, brand_id
 limit 100;

--= query_53
PRAGMA AnsiImplicitCrossJoin;
select  * from 
(select item.i_manufact_id AS i_manufact_id,
  sum(store_sales.ss_sales_price) AS sum_sales,
  avg(sum(store_sales.ss_sales_price)) over (partition by item.i_manufact_id) AS avg_quarterly_sales
 from item, store_sales, date_dim, store
where ss_item_sk = i_item_sk and
ss_sold_date_sk = d_date_sk and
ss_store_sk = s_store_sk and
d_month_seq in (1186,1186+1,1186+2,1186+3,1186+4,1186+5,1186+6,1186+7,1186+8,1186+9,1186+10,1186+11) and
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
order by avg_quarterly_sales,
	 sum_sales,
	 i_manufact_id
limit 100;

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
         and i_category = 'Music'
         and i_class = 'country'
         and c_customer_sk = cs_or_ws_sales.customer_sk
         and d_moy = 1
         and d_year = 1999);
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
                                 from   date_dim where d_year = 1999 and d_moy = 1)
                           and  (select distinct d_month_seq+3
                                 from   date_dim where d_year = 1999 and d_moy = 1)
 group by my_customers.c_customer_sk
 );
$segments = (select cast((revenue/50) as Int32) as segment
  from   $my_revenue AS my_revenue);

  select segment AS segment,
  count(*) AS num_customers,
  segment*50 AS segment_base
 from $segments AS segments
 group by segment
 order by segment, num_customers
 limit 100;

--= query_55
PRAGMA AnsiImplicitCrossJoin;
select item.i_brand_id AS brand_id,
  item.i_brand AS brand,
  sum(store_sales.ss_ext_sales_price) AS ext_price
 from date_dim, store_sales, item
 where d_date_sk = ss_sold_date_sk
 	and ss_item_sk = i_item_sk
 	and i_manager_id=52
 	and d_moy=11
 	and d_year=2000
 group by item.i_brand, item.i_brand_id
 order by ext_price desc, brand_id
 limit 100;

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
where i_color in ('powder','orchid','pink'))
 and     ss_item_sk              = i_item_sk
 and     ss_sold_date_sk         = d_date_sk
 and     d_year                  = 2000
 and     d_moy                   = 3
 and     ss_addr_sk              = ca_address_sk
 and     ca_gmt_offset           = -6 
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
where i_color in ('powder','orchid','pink'))
 and     cs_item_sk              = i_item_sk
 and     cs_sold_date_sk         = d_date_sk
 and     d_year                  = 2000
 and     d_moy                   = 3
 and     cs_bill_addr_sk         = ca_address_sk
 and     ca_gmt_offset           = -6 
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
where i_color in ('powder','orchid','pink'))
 and     ws_item_sk              = i_item_sk
 and     ws_sold_date_sk         = d_date_sk
 and     d_year                  = 2000
 and     d_moy                   = 3
 and     ws_bill_addr_sk         = ca_address_sk
 and     ca_gmt_offset           = -6
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
 order by total_sales, i_item_id
 limit 100;

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
           order by date_dim.d_year, date_dim.d_moy) AS rn
 from item, catalog_sales, date_dim, call_center
 where cs_item_sk = i_item_sk and
       cs_sold_date_sk = d_date_sk and
       cc_call_center_sk= cs_call_center_sk and
       (
         d_year = 2001 or
         ( d_year = 2001-1 and d_moy =12) or
         ( d_year = 2001+1 and d_moy =1)
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

  select  *
 from $v2 AS v2
 where  d_year = 2001 and
        avg_monthly_sales > 0 and
        case when avg_monthly_sales > 0 then abs(sum_sales - avg_monthly_sales) / avg_monthly_sales else null end > 0.1
 order by sum_sales - avg_monthly_sales, avg_monthly_sales
 limit 100;

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
                                      where d_date = '2001-06-16'))
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
                                      where d_date = '2001-06-16'))
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
                                     where d_date = '2001-06-16'))
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
 order by item_id
         ,ss_item_rev
 limit 100;

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
        d_month_seq between 1195 and 1195 + 11) y,
 (select s_store_name s_store_name2,wss.d_week_seq d_week_seq2
        ,s_store_id s_store_id2,sun_sales sun_sales2
        ,mon_sales mon_sales2,tue_sales tue_sales2
        ,wed_sales wed_sales2,thu_sales thu_sales2
        ,fri_sales fri_sales2,sat_sales sat_sales2
  from $wss AS wss,store,date_dim d
  where d.d_week_seq = wss.d_week_seq and
        ss_store_sk = s_store_sk and 
        d_month_seq between 1195+ 12 and 1195 + 23) x
 where s_store_id1=s_store_id2
   and d_week_seq1=d_week_seq2-52
 order by s_store_name1,s_store_id1,d_week_seq1
limit 100;

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
where i_category in ('Jewelry'))
 and     ss_item_sk              = i_item_sk
 and     ss_sold_date_sk         = d_date_sk
 and     d_year                  = 2000
 and     d_moy                   = 10
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
where i_category in ('Jewelry'))
 and     cs_item_sk              = i_item_sk
 and     cs_sold_date_sk         = d_date_sk
 and     d_year                  = 2000
 and     d_moy                   = 10
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
where i_category in ('Jewelry'))
 and     ws_item_sk              = i_item_sk
 and     ws_sold_date_sk         = d_date_sk
 and     d_year                  = 2000
 and     d_moy                   = 10
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
 order by i_item_id, total_sales
 limit 100;

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
   and   ca_gmt_offset = -7
   and   i_category = 'Home'
   and   (p_channel_dmail = 'Y' or p_channel_email = 'Y' or p_channel_tv = 'Y')
   and   s_gmt_offset = -7
   and   d_year = 2000
   and   d_moy  = 12) promotional_sales,
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
   and   ca_gmt_offset = -7
   and   i_category = 'Home'
   and   s_gmt_offset = -7
   and   d_year = 2000
   and   d_moy  = 12) all_sales
order by promotions, total
limit 100;

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
    d_month_seq between 1223 and 1223 + 11
and ws_ship_date_sk   = d_date_sk
and ws_warehouse_sk   = w_warehouse_sk
and ws_ship_mode_sk   = sm_ship_mode_sk
and ws_web_site_sk    = web_site_sk
group by Unicode::Substring(warehouse.w_warehouse_name, CAST(0 AS Uint32), CAST(20 AS Uint32)) AS gk12, ship_mode.sm_type, web_site.web_name
 order by gk12, sm_type, web_name
 limit 100;

--= query_63
PRAGMA AnsiImplicitCrossJoin;
select  * 
from (select item.i_manager_id AS i_manager_id,
  sum(store_sales.ss_sales_price) AS sum_sales,
  avg(sum(store_sales.ss_sales_price)) over (partition by item.i_manager_id) AS avg_monthly_sales
 from item
          ,store_sales
          ,date_dim
          ,store
      where ss_item_sk = i_item_sk
        and ss_sold_date_sk = d_date_sk
        and ss_store_sk = s_store_sk
        and d_month_seq in (1222,1222+1,1222+2,1222+3,1222+4,1222+5,1222+6,1222+7,1222+8,1222+9,1222+10,1222+11)
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
order by i_manager_id
        ,avg_monthly_sales
        ,sum_sales
limit 100;

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
         i_color in ('orange','lace','lawn','misty','blush','pink') and
         i_current_price between 48 and 48 + 10 and
         i_current_price between 48 + 1 and 48 + 15
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
order by cs1.product_name
       ,cs1.store_name
       ,cs2.cnt
       ,cs1.s1
       ,cs2.s1;

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
 order by s_store_name, i_item_desc
limit 100;

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
  'ORIENTAL' || ',' || 'BOXBUNDLES' AS ship_carriers,
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
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS jan_net,
  sum(case when date_dim.d_moy = 2
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS feb_net,
  sum(case when date_dim.d_moy = 3 
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS mar_net,
  sum(case when date_dim.d_moy = 4 
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS apr_net,
  sum(case when date_dim.d_moy = 5 
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS may_net,
  sum(case when date_dim.d_moy = 6 
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS jun_net,
  sum(case when date_dim.d_moy = 7 
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS jul_net,
  sum(case when date_dim.d_moy = 8 
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS aug_net,
  sum(case when date_dim.d_moy = 9 
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS sep_net,
  sum(case when date_dim.d_moy = 10 
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS oct_net,
  sum(case when date_dim.d_moy = 11
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS nov_net,
  sum(case when date_dim.d_moy = 12
 		then web_sales.ws_net_paid_inc_ship * web_sales.ws_quantity else 0 end) AS dec_net
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
 	and t_time between 42970 and 42970+28800 
 	and sm_carrier in ('ORIENTAL','BOXBUNDLES')
     group by warehouse.w_warehouse_name, warehouse.w_warehouse_sq_ft, warehouse.w_city, warehouse.w_county, warehouse.w_state, warehouse.w_country, date_dim.d_year
 union all
     select warehouse.w_warehouse_name AS w_warehouse_name,
  warehouse.w_warehouse_sq_ft AS w_warehouse_sq_ft,
  warehouse.w_city AS w_city,
  warehouse.w_county AS w_county,
  warehouse.w_state AS w_state,
  warehouse.w_country AS w_country,
  'ORIENTAL' || ',' || 'BOXBUNDLES' AS ship_carriers,
  date_dim.d_year AS year,
  sum(case when date_dim.d_moy = 1 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS jan_sales,
  sum(case when date_dim.d_moy = 2 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS feb_sales,
  sum(case when date_dim.d_moy = 3 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS mar_sales,
  sum(case when date_dim.d_moy = 4 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS apr_sales,
  sum(case when date_dim.d_moy = 5 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS may_sales,
  sum(case when date_dim.d_moy = 6 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS jun_sales,
  sum(case when date_dim.d_moy = 7 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS jul_sales,
  sum(case when date_dim.d_moy = 8 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS aug_sales,
  sum(case when date_dim.d_moy = 9 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS sep_sales,
  sum(case when date_dim.d_moy = 10 
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS oct_sales,
  sum(case when date_dim.d_moy = 11
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS nov_sales,
  sum(case when date_dim.d_moy = 12
 		then catalog_sales.cs_ext_list_price* catalog_sales.cs_quantity else 0 end) AS dec_sales,
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
 	and t_time between 42970 AND 42970+28800 
 	and sm_carrier in ('ORIENTAL','BOXBUNDLES')
     group by warehouse.w_warehouse_name, warehouse.w_warehouse_sq_ft, warehouse.w_city, warehouse.w_county, warehouse.w_state, warehouse.w_country, date_dim.d_year
 ) x
 group by w_warehouse_name, w_warehouse_sq_ft, w_city, w_county, w_state, w_country, ship_carriers, year
 order by w_warehouse_name
 limit 100;

--= query_67
PRAGMA AnsiImplicitCrossJoin;
select  *
from (select i_category
            ,i_class
            ,i_brand
            ,i_product_name
            ,d_year
            ,d_qoy
            ,d_moy
            ,s_store_id
            ,sumsales
            ,rank() over (partition by i_category order by sumsales desc) rk
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
          and d_month_seq between 1217 and 1217+11
       group by   rollup(item.i_category, item.i_class, item.i_brand, item.i_product_name, date_dim.d_year, date_dim.d_qoy, date_dim.d_moy,store.s_store_id)
 )dw1) dw2
where rk <= 100
order by i_category
        ,i_class
        ,i_brand
        ,i_product_name
        ,d_year
        ,d_qoy
        ,d_moy
        ,s_store_id
        ,sumsales
        ,rk
limit 100;

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
        and (household_demographics.hd_dep_count = 3 or
             household_demographics.hd_vehicle_count= 4)
        and date_dim.d_year in (1998,1998+1,1998+2)
        and store.s_city in ('Fairview','Midway')
       group by store_sales.ss_ticket_number, store_sales.ss_customer_sk, store_sales.ss_addr_sk, customer_address.ca_city
 ) dn
      ,customer
      ,customer_address current_addr
 where ss_customer_sk = c_customer_sk
   and customer.c_current_addr_sk = current_addr.ca_address_sk
   and current_addr.ca_city <> bought_city
 order by c_last_name
         ,ss_ticket_number
 limit 100;

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
  ca_state in ('IL','TX','ME') and
  cd_demo_sk = c.c_current_cdemo_sk and 
  c.c_customer_sk in (select ss_customer_sk from store_sales,date_dim where ss_sold_date_sk = d_date_sk and
                d_year = 2002 and
                d_moy between 1 and 1+2) and
   (c.c_customer_sk not in (select ws_bill_customer_sk from web_sales,date_dim where ws_sold_date_sk = d_date_sk and
                  d_year = 2002 and
                  d_moy between 1 and 1+2) and
    c.c_customer_sk not in (select cs_ship_customer_sk from catalog_sales,date_dim where cs_sold_date_sk = d_date_sk and
                  d_year = 2002 and
                  d_moy between 1 and 1+2))
 group by customer_demographics.cd_gender, customer_demographics.cd_marital_status, customer_demographics.cd_education_status, customer_demographics.cd_purchase_estimate, customer_demographics.cd_credit_rating
 order by cd_gender, cd_marital_status, cd_education_status, cd_purchase_estimate, cd_credit_rating
 limit 100;

--= query_70
PRAGMA AnsiImplicitCrossJoin;
select sum(store_sales.ss_net_profit) AS total_sum,
  store.s_state AS s_state,
  store.s_county AS s_county,
  grouping(store.s_state)+grouping(store.s_county) AS lochierarchy,
  rank() over (
 	partition by grouping(store.s_state)+grouping(store.s_county),
 	case when grouping(store.s_county) = 0 then store.s_state  else null end 
 	order by sum(store_sales.ss_net_profit) desc) AS rank_within_parent
 from
    store_sales
   ,date_dim       d1
   ,store
 where
    d1.d_month_seq between 1220 and 1220+11
 and d1.d_date_sk = ss_sold_date_sk
 and s_store_sk  = ss_store_sk
 and s_state in
             ( select s_state
               from  (select store.s_state AS s_state,
  rank() over ( partition by store.s_state order by sum(store_sales.ss_net_profit) desc) AS ranking
 from   store_sales, store, date_dim
                      where  d_month_seq between 1220 and 1220+11
 			    and d_date_sk = ss_sold_date_sk
 			    and s_store_sk  = ss_store_sk
                      group by store.s_state
 ) tmp1 
               where ranking <= 5
             )
 group by  rollup(store.s_state,store.s_county)
 
 order by lochierarchy desc, case when lochierarchy = 0 then s_state  else null end, rank_within_parent
 limit 100;

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
                   and d_moy=12
                   and d_year=2002
                 union all
                 select cs_ext_sales_price as ext_price,
                        cs_sold_date_sk as sold_date_sk,
                        cs_item_sk as sold_item_sk,
                        cs_sold_time_sk as time_sk
                 from catalog_sales,date_dim
                 where d_date_sk = cs_sold_date_sk
                   and d_moy=12
                   and d_year=2002
                 union all
                 select ss_ext_sales_price as ext_price,
                        ss_sold_date_sk as sold_date_sk,
                        ss_item_sk as sold_item_sk,
                        ss_sold_time_sk as time_sk
                 from store_sales,date_dim
                 where d_date_sk = ss_sold_date_sk
                   and d_moy=12
                   and d_year=2002
                 ) tmp,time_dim
 where
   sold_item_sk = i_item_sk
   and i_manager_id=1
   and time_sk = t_time_sk
   and (t_meal_time = 'breakfast' or t_meal_time = 'dinner')
 group by item.i_brand, item.i_brand_id, time_dim.t_hour, time_dim.t_minute
 order by ext_price desc, brand_id;

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
  and hd_buy_potential = '1001-5000'
  and d1.d_year = 1998
  and cd_marital_status = 'S'
group by item.i_item_desc, warehouse.w_warehouse_name, d1.d_week_seq
 order by total_cnt desc, i_item_desc, w_warehouse_name, d_week_seq
 limit 100;

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
    and (household_demographics.hd_buy_potential = '1001-5000' or
         household_demographics.hd_buy_potential = '5001-10000')
    and household_demographics.hd_vehicle_count > 0
    and case when household_demographics.hd_vehicle_count > 0 then 
             household_demographics.hd_dep_count/ household_demographics.hd_vehicle_count else null end > 1
    and date_dim.d_year in (2000,2000+1,2000+2)
    and store.s_county in ('Williamson County','Williamson County','Williamson County','Williamson County')
    group by store_sales.ss_ticket_number, store_sales.ss_customer_sk
 ) dj,customer
    where ss_customer_sk = c_customer_sk
      and cnt between 1 and 5
    order by cnt desc, c_last_name asc;

--= query_74
PRAGMA AnsiImplicitCrossJoin;
$year_total = (select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  date_dim.d_year AS year,
  max(store_sales.ss_net_paid) AS year_total,
  's' AS sale_type
 from customer
     ,store_sales
     ,date_dim
 where c_customer_sk = ss_customer_sk
   and ss_sold_date_sk = d_date_sk
   and d_year in (1999,1999+1)
 group by customer.c_customer_id, customer.c_first_name, customer.c_last_name, date_dim.d_year
 union all
 select customer.c_customer_id AS customer_id,
  customer.c_first_name AS customer_first_name,
  customer.c_last_name AS customer_last_name,
  date_dim.d_year AS year,
  max(web_sales.ws_net_paid) AS year_total,
  'w' AS sale_type
 from customer
     ,web_sales
     ,date_dim
 where c_customer_sk = ws_bill_customer_sk
   and ws_sold_date_sk = d_date_sk
   and d_year in (1999,1999+1)
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
         and t_s_firstyear.year = 1999
         and t_s_secyear.year = 1999+1
         and t_w_firstyear.year = 1999
         and t_w_secyear.year = 1999+1
         and t_s_firstyear.year_total > 0
         and t_w_firstyear.year_total > 0
         and case when t_w_firstyear.year_total > 0 then t_w_secyear.year_total / t_w_firstyear.year_total else null end
           > case when t_s_firstyear.year_total > 0 then t_s_secyear.year_total / t_s_firstyear.year_total else null end
 order by customer_id, customer_last_name, customer_first_name
 limit 100;

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
       WHERE i_category='Sports'
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
       WHERE i_category='Sports'
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
       WHERE i_category='Sports') sales_detail
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
 ORDER BY sales_cnt_diff,sales_amt_diff
 limit 100;

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
        SELECT 'store' as channel, 'ss_customer_sk' col_name, d_year, d_qoy, i_category, ss_ext_sales_price ext_sales_price
         FROM store_sales, item, date_dim
         WHERE ss_customer_sk IS NULL
           AND ss_sold_date_sk=d_date_sk
           AND ss_item_sk=i_item_sk
        UNION ALL
        SELECT 'web' as channel, 'ws_promo_sk' col_name, d_year, d_qoy, i_category, ws_ext_sales_price ext_sales_price
         FROM web_sales, item, date_dim
         WHERE ws_promo_sk IS NULL
           AND ws_sold_date_sk=d_date_sk
           AND ws_item_sk=i_item_sk
        UNION ALL
        SELECT 'catalog' as channel, 'cs_bill_customer_sk' col_name, d_year, d_qoy, i_category, cs_ext_sales_price ext_sales_price
         FROM catalog_sales, item, date_dim
         WHERE cs_bill_customer_sk IS NULL
           AND cs_sold_date_sk=d_date_sk
           AND cs_item_sk=i_item_sk) foo
GROUP BY channel, col_name, d_year, d_qoy, i_category
 ORDER BY channel, col_name, d_year, d_qoy, i_category
 limit 100;

--= query_77
PRAGMA AnsiImplicitCrossJoin;
$ss = (select store.s_store_sk AS s_store_sk,
  sum(store_sales.ss_ext_sales_price) AS sales,
  sum(store_sales.ss_net_profit) AS profit
 from store_sales,
      date_dim,
      store
 where ss_sold_date_sk = d_date_sk
       and d_date between ('2000-08-10') 
                  and '2000-09-09' 
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
       and d_date between ('2000-08-10')
                  and '2000-09-09'
       and sr_store_sk = s_store_sk
 group by store.s_store_sk
 );
$cs = (select catalog_sales.cs_call_center_sk AS cs_call_center_sk,
  sum(catalog_sales.cs_ext_sales_price) AS sales,
  sum(catalog_sales.cs_net_profit) AS profit
 from catalog_sales,
      date_dim
 where cs_sold_date_sk = d_date_sk
       and d_date between ('2000-08-10')
                  and '2000-09-09'
 group by catalog_sales.cs_call_center_sk
 );
$cr = (select catalog_returns.cr_call_center_sk AS cr_call_center_sk,
  sum(catalog_returns.cr_return_amount) AS returns,
  sum(catalog_returns.cr_net_loss) AS profit_loss
 from catalog_returns,
      date_dim
 where cr_returned_date_sk = d_date_sk
       and d_date between ('2000-08-10')
                  and '2000-09-09'
 group by catalog_returns.cr_call_center_sk
 );
$ws = (select web_page.wp_web_page_sk AS wp_web_page_sk,
  sum(web_sales.ws_ext_sales_price) AS sales,
  sum(web_sales.ws_net_profit) AS profit
 from web_sales,
      date_dim,
      web_page
 where ws_sold_date_sk = d_date_sk
       and d_date between ('2000-08-10')
                  and '2000-09-09'
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
       and d_date between ('2000-08-10')
                  and '2000-09-09'
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
 
 order by channel, id
 limit 100;

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
where (coalesce(ws_qty,0)>0 or coalesce(cs_qty, 0)>0) and ss_sold_year=1998
order by 
  ss_customer_sk,
  ss_qty desc, ss_wc desc, ss_sp desc,
  other_chan_qty,
  other_chan_wholesale_cost,
  other_chan_sales_price,
  ratio
limit 100;

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
    and (household_demographics.hd_dep_count = 7 or household_demographics.hd_vehicle_count > -1)
    and date_dim.d_dow = 1
    and date_dim.d_year in (2000,2000+1,2000+2) 
    and store.s_number_employees between 200 and 295
    group by store_sales.ss_ticket_number, store_sales.ss_customer_sk, store_sales.ss_addr_sk, store.s_city
 ) ms,customer
    where ss_customer_sk = c_customer_sk
 order by c_last_name,c_first_name,Unicode::Substring(s_city, CAST(0 AS Uint32), CAST(30 AS Uint32)), profit
limit 100;

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
       and d_date between ('2002-08-14') 
                  and '2002-09-13'
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
       and d_date between ('2002-08-14')
                  and '2002-09-13'
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
       and d_date between ('2002-08-14')
                  and '2002-09-13'
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
 
 order by channel, id
 limit 100;

--= query_81
PRAGMA AnsiImplicitCrossJoin;
$customer_total_return = (select catalog_returns.cr_returning_customer_sk AS ctr_customer_sk,
  customer_address.ca_state AS ctr_state,
  sum(catalog_returns.cr_return_amt_inc_tax) AS ctr_total_return
 from catalog_returns
     ,date_dim
     ,customer_address
 where cr_returned_date_sk = d_date_sk 
   and d_year =2001
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
       and ca_state = 'TN'
       and ctr1.ctr_customer_sk = c_customer_sk
 order by c_customer_id,c_salutation,c_first_name,c_last_name,ca_street_number,ca_street_name
                   ,ca_street_type,ca_suite_number,ca_city,ca_county,ca_state,ca_zip,ca_country,ca_gmt_offset
                  ,ca_location_type,ctr_total_return
 limit 100;

--= query_82
PRAGMA AnsiImplicitCrossJoin;
select item.i_item_id AS i_item_id,
  item.i_item_desc AS i_item_desc,
  item.i_current_price AS i_current_price
 from item, inventory, date_dim, store_sales
 where i_current_price between 58 and 58+30
 and inv_item_sk = i_item_sk
 and d_date_sk=inv_date_sk
 and d_date between ('2001-01-13') and '2001-03-14'
 and i_manufact_id in (259,559,580,485)
 and inv_quantity_on_hand between 100 and 500
 and ss_item_sk = i_item_sk
 group by item.i_item_id, item.i_item_desc, item.i_current_price
 order by i_item_id
 limit 100;

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
	  where d_date in ('2001-07-13','2001-09-10','2001-11-16')))
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
	  where d_date in ('2001-07-13','2001-09-10','2001-11-16')))
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
		where d_date in ('2001-07-13','2001-09-10','2001-11-16')))
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
 order by sr_items.item_id
         ,sr_item_qty
 limit 100;

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
 where ca_city	        =  'Woodland'
   and c_current_addr_sk = ca_address_sk
   and ib_lower_bound   >=  60306
   and ib_upper_bound   <=  60306 + 50000
   and ib_income_band_sk = hd_income_band_sk
   and cd_demo_sk = c_current_cdemo_sk
   and hd_demo_sk = c_current_hdemo_sk
   and sr_cdemo_sk = cd_demo_sk
 order by c_customer_id
 limit 100;

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
   and ws_sold_date_sk = d_date_sk and d_year = 1998
   and cd1.cd_demo_sk = wr_refunded_cdemo_sk 
   and cd2.cd_demo_sk = wr_returning_cdemo_sk
   and ca_address_sk = wr_refunded_addr_sk
   and r_reason_sk = wr_reason_sk
   and
   (
    (
     cd1.cd_marital_status = 'D'
     and
     cd1.cd_marital_status = cd2.cd_marital_status
     and
     cd1.cd_education_status = 'Primary'
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
     cd1.cd_marital_status = 'U'
     and
     cd1.cd_marital_status = cd2.cd_marital_status
     and
     cd1.cd_education_status = 'Advanced Degree'
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
     ca_state in ('NC', 'TX', 'IA')
     and ws_net_profit between 100 and 200  
    )
    or
    (
     ca_country = 'United States'
     and
     ca_state in ('WI', 'WV', 'GA')
     and ws_net_profit between 150 and 300  
    )
    or
    (
     ca_country = 'United States'
     and
     ca_state in ('OK', 'VA', 'KY')
     and ws_net_profit between 50 and 250  
    )
   )
group by reason.r_reason_desc
 order by c13, c14, c15, c16
 limit 100;

--= query_86
PRAGMA AnsiImplicitCrossJoin;
select sum(web_sales.ws_net_paid) AS total_sum,
  item.i_category AS i_category,
  item.i_class AS i_class,
  grouping(item.i_category)+grouping(item.i_class) AS lochierarchy,
  rank() over (
 	partition by grouping(item.i_category)+grouping(item.i_class),
 	case when grouping(item.i_class) = 0 then item.i_category  else null end 
 	order by sum(web_sales.ws_net_paid) desc) AS rank_within_parent
 from
    web_sales
   ,date_dim       d1
   ,item
 where
    d1.d_month_seq between 1186 and 1186+11
 and d1.d_date_sk = ws_sold_date_sk
 and i_item_sk  = ws_item_sk
 group by  rollup(item.i_category,item.i_class)
 
 order by lochierarchy desc, case when lochierarchy = 0 then i_category  else null end, rank_within_parent
 limit 100;

--= query_87
PRAGMA AnsiImplicitCrossJoin;
select count(*) 
from ((select distinct c_last_name, c_first_name, d_date
       from store_sales, date_dim, customer
       where store_sales.ss_sold_date_sk = date_dim.d_date_sk
         and store_sales.ss_customer_sk = customer.c_customer_sk
         and d_month_seq between 1202 and 1202+11)
       except
      (select distinct c_last_name, c_first_name, d_date
       from catalog_sales, date_dim, customer
       where catalog_sales.cs_sold_date_sk = date_dim.d_date_sk
         and catalog_sales.cs_bill_customer_sk = customer.c_customer_sk
         and d_month_seq between 1202 and 1202+11)
       except
      (select distinct c_last_name, c_first_name, d_date
       from web_sales, date_dim, customer
       where web_sales.ws_sold_date_sk = date_dim.d_date_sk
         and web_sales.ws_bill_customer_sk = customer.c_customer_sk
         and d_month_seq between 1202 and 1202+11)
) cool_cust;

--= query_88
PRAGMA AnsiImplicitCrossJoin;
select  *
from
 (select count(*) AS h8_30_to_9
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk   
     and ss_hdemo_sk = household_demographics.hd_demo_sk 
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 8
     and time_dim.t_minute >= 30
     and ((household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 3 and household_demographics.hd_vehicle_count<=3+2)) 
     and store.s_store_name = 'ese') s1,
 (select count(*) AS h9_to_9_30
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk 
     and time_dim.t_hour = 9 
     and time_dim.t_minute < 30
     and ((household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 3 and household_demographics.hd_vehicle_count<=3+2))
     and store.s_store_name = 'ese') s2,
 (select count(*) AS h9_30_to_10
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 9
     and time_dim.t_minute >= 30
     and ((household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 3 and household_demographics.hd_vehicle_count<=3+2))
     and store.s_store_name = 'ese') s3,
 (select count(*) AS h10_to_10_30
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 10 
     and time_dim.t_minute < 30
     and ((household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 3 and household_demographics.hd_vehicle_count<=3+2))
     and store.s_store_name = 'ese') s4,
 (select count(*) AS h10_30_to_11
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 10 
     and time_dim.t_minute >= 30
     and ((household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 3 and household_demographics.hd_vehicle_count<=3+2))
     and store.s_store_name = 'ese') s5,
 (select count(*) AS h11_to_11_30
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk 
     and time_dim.t_hour = 11
     and time_dim.t_minute < 30
     and ((household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 3 and household_demographics.hd_vehicle_count<=3+2))
     and store.s_store_name = 'ese') s6,
 (select count(*) AS h11_30_to_12
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 11
     and time_dim.t_minute >= 30
     and ((household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 3 and household_demographics.hd_vehicle_count<=3+2))
     and store.s_store_name = 'ese') s7,
 (select count(*) AS h12_to_12_30
 from store_sales, household_demographics , time_dim, store
 where ss_sold_time_sk = time_dim.t_time_sk
     and ss_hdemo_sk = household_demographics.hd_demo_sk
     and ss_store_sk = s_store_sk
     and time_dim.t_hour = 12
     and time_dim.t_minute < 30
     and ((household_demographics.hd_dep_count = 0 and household_demographics.hd_vehicle_count<=0+2) or
          (household_demographics.hd_dep_count = -1 and household_demographics.hd_vehicle_count<=-1+2) or
          (household_demographics.hd_dep_count = 3 and household_demographics.hd_vehicle_count<=3+2))
     and store.s_store_name = 'ese') s8;

--= query_89
PRAGMA AnsiImplicitCrossJoin;
select  *
from(
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
      d_year in (2001) and
        ((i_category in ('Books','Children','Electronics') and
          i_class in ('history','school-uniforms','audio')
         )
      or (i_category in ('Men','Sports','Shoes') and
          i_class in ('pants','tennis','womens') 
        ))
group by item.i_category, item.i_class, item.i_brand, store.s_store_name, store.s_company_name, date_dim.d_moy
 ) tmp1
where case when (avg_monthly_sales <> 0) then (abs(sum_sales - avg_monthly_sales) / avg_monthly_sales) else null end > 0.1
order by sum_sales - avg_monthly_sales, s_store_name
limit 100;

--= query_90
PRAGMA AnsiImplicitCrossJoin;
select  cast(amc as Double)/nullif(cast(pmc as Double),0) am_pm_ratio
 from ( select count(*) AS amc
 from web_sales, household_demographics , time_dim, web_page
       where ws_sold_time_sk = time_dim.t_time_sk
         and ws_ship_hdemo_sk = household_demographics.hd_demo_sk
         and ws_web_page_sk = web_page.wp_web_page_sk
         and time_dim.t_hour between 12 and 12+1
         and household_demographics.hd_dep_count = 6
         and web_page.wp_char_count between 5000 and 5200) at,
      ( select count(*) AS pmc
 from web_sales, household_demographics , time_dim, web_page
       where ws_sold_time_sk = time_dim.t_time_sk
         and ws_ship_hdemo_sk = household_demographics.hd_demo_sk
         and ws_web_page_sk = web_page.wp_web_page_sk
         and time_dim.t_hour between 14 and 14+1
         and household_demographics.hd_dep_count = 6
         and web_page.wp_char_count between 5000 and 5200) pt
 order by am_pm_ratio
 limit 100;

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
and     d_year                  = 2000 
and     d_moy                   = 12
and     ( (cd_marital_status       = 'M' and cd_education_status     = 'Unknown')
        or(cd_marital_status       = 'W' and cd_education_status     = 'Advanced Degree'))
and     hd_buy_potential like 'Unknown%'
and     ca_gmt_offset           = -7
group by call_center.cc_call_center_id, call_center.cc_name, call_center.cc_manager, customer_demographics.cd_marital_status, customer_demographics.cd_education_status
 order by Returns_Loss desc;

--= query_92
PRAGMA AnsiImplicitCrossJoin;
$disc = (select web_sales.ws_item_sk AS ditem,
  1.3 * avg(web_sales.ws_ext_discount_amt) AS avg_disc
 from web_sales, date_dim
  where d_date between '2000-02-01' and '2000-05-01'
    and d_date_sk = ws_sold_date_sk
  group by web_sales.ws_item_sk
 );

select sum(web_sales.ws_ext_discount_amt) AS `Excess Discount Amount`
 from web_sales, item, date_dim, $disc AS disc
where i_manufact_id = 714
  and i_item_sk = ws_item_sk
  and d_date between '2000-02-01' and '2000-05-01'
  and d_date_sk = ws_sold_date_sk
  and disc.ditem = web_sales.ws_item_sk
  and web_sales.ws_ext_discount_amt > disc.avg_disc
order by `Excess Discount Amount`
 limit 100;

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
              and r_reason_desc = 'reason 58') t
      group by ss_customer_sk
 order by sumsales, ss_customer_sk
 limit 100;

--= query_94
PRAGMA AnsiImplicitCrossJoin;
select count(distinct ws1.ws_order_number) AS `order count`,
  sum(ws1.ws_ext_ship_cost) AS `total shipping cost`,
  sum(ws1.ws_net_profit) AS `total net profit`
 from web_sales ws1, date_dim, customer_address, web_site
where d_date between '2002-05-01' and '2002-06-30'
  and ws1.ws_ship_date_sk = d_date_sk
  and ws1.ws_ship_addr_sk = ca_address_sk
  and ca_state = 'OK'
  and ws1.ws_web_site_sk = web_site_sk
  and web_company_name = 'pri'
  and ws1.ws_order_number in (select ws_order_number AS ws_order_number
 from web_sales group by ws_order_number
 having count(distinct ws_warehouse_sk) > 1)
  and ws1.ws_order_number not in (select wr_order_number from web_returns)
order by `order count`
 limit 100;

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
    d_date between '2001-04-01' and 
           '2001-05-31'
and ws1.ws_ship_date_sk = d_date_sk
and ws1.ws_ship_addr_sk = ca_address_sk
and ca_state = 'VA'
and ws1.ws_web_site_sk = web_site_sk
and web_company_name = 'pri'
and ws1.ws_order_number in (select ws_order_number
                            from $ws_wh AS ws_wh)
and ws1.ws_order_number in (select wr_order_number
                            from web_returns,$ws_wh AS ws_wh
                            where wr_order_number = ws_wh.ws_order_number)
order by `order count`
 limit 100;

--= query_96
PRAGMA AnsiImplicitCrossJoin;
select count(*) AS c17
 from store_sales
    ,household_demographics 
    ,time_dim, store
where ss_sold_time_sk = time_dim.t_time_sk   
    and ss_hdemo_sk = household_demographics.hd_demo_sk 
    and ss_store_sk = s_store_sk
    and time_dim.t_hour = 8
    and time_dim.t_minute >= 30
    and household_demographics.hd_dep_count = 0
    and store.s_store_name = 'ese'
order by c17
 limit 100;

--= query_97
PRAGMA AnsiImplicitCrossJoin;
$ssci = (select store_sales.ss_customer_sk AS customer_sk,
  store_sales.ss_item_sk AS item_sk
 from store_sales,date_dim
where ss_sold_date_sk = d_date_sk
  and d_month_seq between 1199 and 1199 + 11
group by store_sales.ss_customer_sk, store_sales.ss_item_sk
 );
$csci = (select catalog_sales.cs_bill_customer_sk AS customer_sk,
  catalog_sales.cs_item_sk AS item_sk
 from catalog_sales,date_dim
where cs_sold_date_sk = d_date_sk
  and d_month_seq between 1199 and 1199 + 11
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
	and d_date between ('1999-02-05') 
				and '1999-03-07'
group by item.i_item_id, item.i_item_desc, item.i_category, item.i_class, item.i_current_price
 order by i_category, i_class, i_item_id, i_item_desc, revenueratio;

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
    d_month_seq between 1194 and 1194 + 11
and cs_ship_date_sk   = d_date_sk
and cs_warehouse_sk   = w_warehouse_sk
and cs_ship_mode_sk   = sm_ship_mode_sk
and cs_call_center_sk = cc_call_center_sk
group by Unicode::Substring(warehouse.w_warehouse_name, CAST(0 AS Uint32), CAST(20 AS Uint32)) AS gk18, ship_mode.sm_type, call_center.cc_name
 order by gk18, sm_type, cc_name
 limit 100;
