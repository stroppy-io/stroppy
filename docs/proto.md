# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [proto/stroppy/cloud.proto](#proto_stroppy_cloud-proto)
    - [StroppyRun](#stroppy-StroppyRun)
    - [StroppyRun.StepsEntry](#stroppy-StroppyRun-StepsEntry)
  
    - [StroppyRun.Status](#stroppy-StroppyRun-Status)
  
    - [CloudStatusService](#stroppy-CloudStatusService)
  
- [proto/stroppy/common.proto](#proto_stroppy_common-proto)
    - [DateTime](#stroppy-DateTime)
    - [Decimal](#stroppy-Decimal)
    - [Generation](#stroppy-Generation)
    - [Generation.Alphabet](#stroppy-Generation-Alphabet)
    - [Generation.Distribution](#stroppy-Generation-Distribution)
    - [Generation.Range](#stroppy-Generation-Range)
    - [Generation.Range.AnyString](#stroppy-Generation-Range-AnyString)
    - [Generation.Range.Bool](#stroppy-Generation-Range-Bool)
    - [Generation.Range.DateTime](#stroppy-Generation-Range-DateTime)
    - [Generation.Range.DateTime.TimestampPb](#stroppy-Generation-Range-DateTime-TimestampPb)
    - [Generation.Range.DateTime.TimestampUnix](#stroppy-Generation-Range-DateTime-TimestampUnix)
    - [Generation.Range.DecimalRange](#stroppy-Generation-Range-DecimalRange)
    - [Generation.Range.Double](#stroppy-Generation-Range-Double)
    - [Generation.Range.Float](#stroppy-Generation-Range-Float)
    - [Generation.Range.Int32](#stroppy-Generation-Range-Int32)
    - [Generation.Range.Int64](#stroppy-Generation-Range-Int64)
    - [Generation.Range.String](#stroppy-Generation-Range-String)
    - [Generation.Range.UInt32](#stroppy-Generation-Range-UInt32)
    - [Generation.Range.UInt64](#stroppy-Generation-Range-UInt64)
    - [Generation.Range.UuidSeq](#stroppy-Generation-Range-UuidSeq)
    - [Generation.Rule](#stroppy-Generation-Rule)
    - [Generation.StringDictionary](#stroppy-Generation-StringDictionary)
    - [Generation.StringLiteralInject](#stroppy-Generation-StringLiteralInject)
    - [Generation.WeightedChoice](#stroppy-Generation-WeightedChoice)
    - [Generation.WeightedChoice.Item](#stroppy-Generation-WeightedChoice-Item)
    - [OtlpExport](#stroppy-OtlpExport)
    - [Uuid](#stroppy-Uuid)
    - [Value](#stroppy-Value)
    - [Value.List](#stroppy-Value-List)
    - [Value.Struct](#stroppy-Value-Struct)
  
    - [Generation.Distribution.DistributionType](#stroppy-Generation-Distribution-DistributionType)
    - [Generation.Distribution.NURandPhase](#stroppy-Generation-Distribution-NURandPhase)
    - [Value.NullValue](#stroppy-Value-NullValue)
  
- [proto/stroppy/config.proto](#proto_stroppy_config-proto)
    - [DriverConfig](#stroppy-DriverConfig)
    - [DriverConfig.PostgresConfig](#stroppy-DriverConfig-PostgresConfig)
    - [DriverConfig.SqlConfig](#stroppy-DriverConfig-SqlConfig)
    - [ExporterConfig](#stroppy-ExporterConfig)
    - [GlobalConfig](#stroppy-GlobalConfig)
    - [GlobalConfig.MetadataEntry](#stroppy-GlobalConfig-MetadataEntry)
    - [LoggerConfig](#stroppy-LoggerConfig)
  
    - [DriverConfig.DriverType](#stroppy-DriverConfig-DriverType)
    - [DriverConfig.ErrorMode](#stroppy-DriverConfig-ErrorMode)
    - [LoggerConfig.LogLevel](#stroppy-LoggerConfig-LogLevel)
    - [LoggerConfig.LogMode](#stroppy-LoggerConfig-LogMode)
  
- [proto/stroppy/datagen.proto](#proto_stroppy_datagen-proto)
    - [AsciiRange](#stroppy-datagen-AsciiRange)
    - [Attr](#stroppy-datagen-Attr)
    - [BinOp](#stroppy-datagen-BinOp)
    - [BlockRef](#stroppy-datagen-BlockRef)
    - [BlockSlot](#stroppy-datagen-BlockSlot)
    - [Call](#stroppy-datagen-Call)
    - [Choose](#stroppy-datagen-Choose)
    - [ChooseBranch](#stroppy-datagen-ChooseBranch)
    - [Cohort](#stroppy-datagen-Cohort)
    - [CohortDraw](#stroppy-datagen-CohortDraw)
    - [CohortLive](#stroppy-datagen-CohortLive)
    - [ColRef](#stroppy-datagen-ColRef)
    - [Degree](#stroppy-datagen-Degree)
    - [DegreeFixed](#stroppy-datagen-DegreeFixed)
    - [DegreeUniform](#stroppy-datagen-DegreeUniform)
    - [Dict](#stroppy-datagen-Dict)
    - [DictAt](#stroppy-datagen-DictAt)
    - [DictRow](#stroppy-datagen-DictRow)
    - [DrawAscii](#stroppy-datagen-DrawAscii)
    - [DrawBernoulli](#stroppy-datagen-DrawBernoulli)
    - [DrawDate](#stroppy-datagen-DrawDate)
    - [DrawDecimal](#stroppy-datagen-DrawDecimal)
    - [DrawDict](#stroppy-datagen-DrawDict)
    - [DrawFloatUniform](#stroppy-datagen-DrawFloatUniform)
    - [DrawIntUniform](#stroppy-datagen-DrawIntUniform)
    - [DrawJoint](#stroppy-datagen-DrawJoint)
    - [DrawNURand](#stroppy-datagen-DrawNURand)
    - [DrawNormal](#stroppy-datagen-DrawNormal)
    - [DrawPhrase](#stroppy-datagen-DrawPhrase)
    - [DrawZipf](#stroppy-datagen-DrawZipf)
    - [Expr](#stroppy-datagen-Expr)
    - [If](#stroppy-datagen-If)
    - [InsertSpec](#stroppy-datagen-InsertSpec)
    - [InsertSpec.DictsEntry](#stroppy-datagen-InsertSpec-DictsEntry)
    - [Literal](#stroppy-datagen-Literal)
    - [Lookup](#stroppy-datagen-Lookup)
    - [LookupPop](#stroppy-datagen-LookupPop)
    - [Null](#stroppy-datagen-Null)
    - [Parallelism](#stroppy-datagen-Parallelism)
    - [Population](#stroppy-datagen-Population)
    - [RelSource](#stroppy-datagen-RelSource)
    - [Relationship](#stroppy-datagen-Relationship)
    - [RowIndex](#stroppy-datagen-RowIndex)
    - [SCD2](#stroppy-datagen-SCD2)
    - [Side](#stroppy-datagen-Side)
    - [Strategy](#stroppy-datagen-Strategy)
    - [StrategyEquitable](#stroppy-datagen-StrategyEquitable)
    - [StrategyHash](#stroppy-datagen-StrategyHash)
    - [StrategySequential](#stroppy-datagen-StrategySequential)
    - [StreamDraw](#stroppy-datagen-StreamDraw)
  
    - [BinOp.Op](#stroppy-datagen-BinOp-Op)
    - [InsertMethod](#stroppy-datagen-InsertMethod)
    - [RowIndex.Kind](#stroppy-datagen-RowIndex-Kind)
  
- [proto/stroppy/descriptor.proto](#proto_stroppy_descriptor-proto)
    - [InsertDescriptor](#stroppy-InsertDescriptor)
    - [QueryParamDescriptor](#stroppy-QueryParamDescriptor)
    - [QueryParamGroup](#stroppy-QueryParamGroup)
  
    - [InsertMethod](#stroppy-InsertMethod)
    - [TxIsolationLevel](#stroppy-TxIsolationLevel)
  
- [proto/stroppy/run.proto](#proto_stroppy_run-proto)
    - [DriverRunConfig](#stroppy-DriverRunConfig)
    - [DriverRunConfig.PoolConfig](#stroppy-DriverRunConfig-PoolConfig)
    - [RunConfig](#stroppy-RunConfig)
    - [RunConfig.DriversEntry](#stroppy-RunConfig-DriversEntry)
    - [RunConfig.EnvEntry](#stroppy-RunConfig-EnvEntry)
  
- [proto/stroppy/runtime.proto](#proto_stroppy_runtime-proto)
    - [DriverQuery](#stroppy-DriverQuery)
    - [DriverQueryStat](#stroppy-DriverQueryStat)
    - [DriverTransaction](#stroppy-DriverTransaction)
    - [DriverTransactionStat](#stroppy-DriverTransactionStat)
  
- [Scalar Value Types](#scalar-value-types)



<a name="proto_stroppy_cloud-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## proto/stroppy/cloud.proto



<a name="stroppy-StroppyRun"></a>

### StroppyRun
StroppyRun represents a benchmark run on the stroppy cli.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| id | [string](#string) |  | Unique identifier for the run generated by stroppy-cli |
| status | [StroppyRun.Status](#stroppy-StroppyRun-Status) |  | Status of the run |
| cmd | [string](#string) |  | Command used to run the benchmark |
| steps | [StroppyRun.StepsEntry](#stroppy-StroppyRun-StepsEntry) | repeated | Additional metadata for the run |






<a name="stroppy-StroppyRun-StepsEntry"></a>

### StroppyRun.StepsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [StroppyRun.Status](#stroppy-StroppyRun-Status) |  |  |





 


<a name="stroppy-StroppyRun-Status"></a>

### StroppyRun.Status


| Name | Number | Description |
| ---- | ------ | ----------- |
| STATUS_IDLE | 0 | Run or step is idle |
| STATUS_RUNNING | 1 | Run or step is running |
| STATUS_COMPLETED | 2 | Run or step has completed successfully |
| STATUS_FAILED | 3 | Run or step has failed |
| STATUS_CANCELLED | 4 | Run or step has been cancelled |


 

 


<a name="stroppy-CloudStatusService"></a>

### CloudStatusService
CloudStatusService is a service for notifying the cloud status of runs and
steps.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| NotifyRun | [StroppyRun](#stroppy-StroppyRun) | [.google.protobuf.Empty](#google-protobuf-Empty) | Notifies the cloud status of a benchmark run |

 



<a name="proto_stroppy_common-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## proto/stroppy/common.proto



<a name="stroppy-DateTime"></a>

### DateTime
DateTime represents a point in time, independent of any time zone or
calendar.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | Timestamp in UTC |






<a name="stroppy-Decimal"></a>

### Decimal
Decimal represents an arbitrary-precision decimal number.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  | String representation of the decimal number |






<a name="stroppy-Generation"></a>

### Generation
Generation contains configuration for generating test data.
It provides rules and constraints for generating various types of data.

UTF-8 character ranges for different languages
Example: {&#34;en&#34;: {{65, 90}, {97, 122}}}






<a name="stroppy-Generation-Alphabet"></a>

### Generation.Alphabet
Alphabet defines character ranges for string generation.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ranges | [Generation.Range.UInt32](#stroppy-Generation-Range-UInt32) | repeated | List of character ranges for this alphabet |






<a name="stroppy-Generation-Distribution"></a>

### Generation.Distribution
Distribution defines the statistical distribution for value generation.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [Generation.Distribution.DistributionType](#stroppy-Generation-Distribution-DistributionType) |  | Type of distribution to use |
| screw | [double](#double) |  | Distribution parameter (e.g., standard deviation for normal distribution, `A` for NURAND) |
| nurand_phase | [Generation.Distribution.NURandPhase](#stroppy-Generation-Distribution-NURandPhase) |  | For NURAND: which phase this generator is for (C-Load or C-Run). Used by §2.1.6.1 / §5.3 audit rule on |C_run - C_load|. |






<a name="stroppy-Generation-Range"></a>

### Generation.Range
Range defines value constraints for generation.






<a name="stroppy-Generation-Range-AnyString"></a>

### Generation.Range.AnyString
Range for string values that can be parsed into other types


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [string](#string) |  | Minimum value (inclusive) |
| max | [string](#string) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-Bool"></a>

### Generation.Range.Bool



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| ratio | [float](#float) |  |  |






<a name="stroppy-Generation-Range-DateTime"></a>

### Generation.Range.DateTime
Range for date/time values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| string | [Generation.Range.AnyString](#stroppy-Generation-Range-AnyString) |  | String-based range (ISO 8601 format) |
| timestamp_pb | [Generation.Range.DateTime.TimestampPb](#stroppy-Generation-Range-DateTime-TimestampPb) |  | Protocol Buffers timestamp range |
| timestamp | [Generation.Range.DateTime.TimestampUnix](#stroppy-Generation-Range-DateTime-TimestampUnix) |  | Unix timestamp range |






<a name="stroppy-Generation-Range-DateTime-TimestampPb"></a>

### Generation.Range.DateTime.TimestampPb
Protocol Buffers timestamp range


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | Minimum timestamp (inclusive) |
| max | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | Maximum timestamp (inclusive) |






<a name="stroppy-Generation-Range-DateTime-TimestampUnix"></a>

### Generation.Range.DateTime.TimestampUnix
Unix timestamp range


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [uint32](#uint32) |  | Minimum Unix timestamp (inclusive) |
| max | [uint32](#uint32) |  | Maximum Unix timestamp (inclusive) |






<a name="stroppy-Generation-Range-DecimalRange"></a>

### Generation.Range.DecimalRange
Range for decimal numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| float | [Generation.Range.Float](#stroppy-Generation-Range-Float) |  | Float-based range |
| double | [Generation.Range.Double](#stroppy-Generation-Range-Double) |  | Double-based range |
| string | [Generation.Range.AnyString](#stroppy-Generation-Range-AnyString) |  | String-bsed range (supports scientific notation) |






<a name="stroppy-Generation-Range-Double"></a>

### Generation.Range.Double
Range for 64-bit floating point numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [double](#double) | optional | Minimum value (inclusive) |
| max | [double](#double) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-Float"></a>

### Generation.Range.Float
Range for 32-bit floating point numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [float](#float) | optional | Minimum value (inclusive) |
| max | [float](#float) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-Int32"></a>

### Generation.Range.Int32
Range for 32-bit signed integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [int32](#int32) | optional | Minimum value (inclusive) |
| max | [int32](#int32) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-Int64"></a>

### Generation.Range.Int64
Range for 64-bit signed integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [int64](#int64) | optional | Minimum value (inclusive) |
| max | [int64](#int64) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-String"></a>

### Generation.Range.String



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| alphabet | [Generation.Alphabet](#stroppy-Generation-Alphabet) | optional | Character set to use for generation |
| min_len | [uint64](#uint64) | optional |  |
| max_len | [uint64](#uint64) |  |  |






<a name="stroppy-Generation-Range-UInt32"></a>

### Generation.Range.UInt32
Range for 32-bit unsigned integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [uint32](#uint32) | optional | Minimum value (inclusive) |
| max | [uint32](#uint32) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-UInt64"></a>

### Generation.Range.UInt64
Range for 64-bit unsigned integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [uint64](#uint64) | optional | Minimum value (inclusive) |
| max | [uint64](#uint64) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-UuidSeq"></a>

### Generation.Range.UuidSeq
Sequential UUID range, counting from min to max.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [Uuid](#stroppy-Uuid) | optional | Start UUID (inclusive); defaults to 00000000-0000-0000-0000-000000000000 if not set |
| max | [Uuid](#stroppy-Uuid) |  | End UUID (inclusive) |






<a name="stroppy-Generation-Rule"></a>

### Generation.Rule
Rule defines generation rules for a specific data type.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| int32_range | [Generation.Range.Int32](#stroppy-Generation-Range-Int32) |  | Signed 32‑bit integer range (inclusive). Example: 1..100 for IDs. |
| int64_range | [Generation.Range.Int64](#stroppy-Generation-Range-Int64) |  | Signed 64‑bit integer range for large counters or timestamps. |
| uint32_range | [Generation.Range.UInt32](#stroppy-Generation-Range-UInt32) |  | Unsigned 32‑bit integer range; use for sizes/indices. |
| uint64_range | [Generation.Range.UInt64](#stroppy-Generation-Range-UInt64) |  | Unsigned 64‑bit integer range; use for large sizes. |
| float_range | [Generation.Range.Float](#stroppy-Generation-Range-Float) |  | 32‑bit float bounds; beware precision for currency. |
| double_range | [Generation.Range.Double](#stroppy-Generation-Range-Double) |  | 64‑bit float bounds for high‑precision numeric data. |
| decimal_range | [Generation.Range.DecimalRange](#stroppy-Generation-Range-DecimalRange) |  | Arbitrary‑precision decimal bounds for money/ratios. |
| string_range | [Generation.Range.String](#stroppy-Generation-Range-String) |  | String constraints (length, alphabet). |
| bool_range | [Generation.Range.Bool](#stroppy-Generation-Range-Bool) |  | Boolean constraints (e.g., force true/false). |
| datetime_range | [Generation.Range.DateTime](#stroppy-Generation-Range-DateTime) |  | Date/time window (e.g., not before/after). |
| int32_const | [int32](#int32) |  | Fixed 32‑bit integer value. |
| int64_const | [int64](#int64) |  | Fixed 64‑bit integer value. |
| uint32_const | [uint32](#uint32) |  | Fixed unsigned 32‑bit integer value. |
| uint64_const | [uint64](#uint64) |  | Fixed unsigned 64‑bit integer value. |
| float_const | [float](#float) |  | Fixed 32‑bit float value. |
| double_const | [double](#double) |  | Fixed 64‑bit float value. |
| decimal_const | [Decimal](#stroppy-Decimal) |  | Fixed decimal value. |
| string_const | [string](#string) |  | Fixed string value. |
| bool_const | [bool](#bool) |  | Fixed boolean value. |
| datetime_const | [DateTime](#stroppy-DateTime) |  | Fixed date/time value. |
| uuid_random | [bool](#bool) |  | Random UUID value (v4). Seed is ignored. |
| uuid_const | [Uuid](#stroppy-Uuid) |  | Fixed UUID value. |
| uuid_seeded | [bool](#bool) |  | Random UUID value (v4) reproducible by seed. |
| uuid_seq | [Generation.Range.UuidSeq](#stroppy-Generation-Range-UuidSeq) |  | Sequential UUIDs from min to max (00000...1 → 00000...N). |
| weighted_choice | [Generation.WeightedChoice](#stroppy-Generation-WeightedChoice) |  | Weighted choice over N sub-rules (e.g., GC/BC string mix). |
| string_dictionary | [Generation.StringDictionary](#stroppy-Generation-StringDictionary) |  | Pick a string from a fixed list by sub-rule index or cycling counter (TPC-C C_LAST §4.3.2.3 syllable dictionary). |
| string_literal_inject | [Generation.StringLiteralInject](#stroppy-Generation-StringLiteralInject) |  | Random string with a literal substring injected at a random position in a percentage of rows (TPC-C I_DATA / S_DATA §4.3.3.1 &#34;ORIGINAL&#34; marker). |
| distribution | [Generation.Distribution](#stroppy-Generation-Distribution) | optional | Shape of randomness; Normal by default; Only for numbers |
| null_percentage | [uint32](#uint32) | optional | Percentage of nulls to inject [0..100]; 0 by default |
| unique | [bool](#bool) | optional | Enforce uniqueness across generated values; Linear sequence for ranges |






<a name="stroppy-Generation-StringDictionary"></a>

### Generation.StringDictionary
StringDictionary picks a string from a fixed list by index. Used for
TPC-C C_LAST (§4.3.2.3) — the 1000-entry syllable dictionary that
indexes sequentially for the first 1000 customers per district and
via NURand(255,0,999) for the remaining 2000.

If `index` is set, the sub-rule produces integer indices on each Next();
values are wrapped modulo len(values). If `index` is omitted, an internal
monotonic counter cycles through `values` on each Next() call — useful
for deterministic sequential traversal with no extra generator setup.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| values | [string](#string) | repeated | Candidate values. At least one required. |
| index | [Generation.Rule](#stroppy-Generation-Rule) | optional | Optional index source. If omitted, an internal counter cycles through values on each Next(). If set, must produce integer values; out-of-range indices are wrapped modulo len(values). |






<a name="stroppy-Generation-StringLiteralInject"></a>

### Generation.StringLiteralInject
StringLiteralInject generates a random string that contains a fixed
literal substring in `inject_percentage` of rows. Used for TPC-C
I_DATA / S_DATA (§4.3.3.1) — 10% of rows must contain the literal
&#34;ORIGINAL&#34; at a random position within the total string length.

On each Next(): draws a length in [min_len, max_len]; with probability
inject_percentage/100 places `literal` at a random offset and fills the
remaining positions with random characters from `alphabet`; otherwise
generates a plain random string of the chosen length.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| literal | [string](#string) |  | The literal substring to inject (e.g., &#34;ORIGINAL&#34;). Must be non-empty. |
| inject_percentage | [uint32](#uint32) |  | Percentage of rows where the literal is injected [0..100]. |
| min_len | [uint64](#uint64) |  | Minimum total string length (must be &gt;= len(literal)). |
| max_len | [uint64](#uint64) |  | Maximum total string length (inclusive; must be &gt;= min_len). |
| alphabet | [Generation.Alphabet](#stroppy-Generation-Alphabet) | optional | Alphabet for non-literal characters. If omitted, falls back to the default English alphabet used by Range.String. |






<a name="stroppy-Generation-WeightedChoice"></a>

### Generation.WeightedChoice
WeightedChoice picks one of N sub-rules with given weights per Next() call.
Useful for mixing categorical values (e.g., TPC-C C_CREDIT = 10% &#34;BC&#34; /
90% &#34;GC&#34;) without coupling two independent generators at the call site.

Weights are relative; they don&#39;t have to sum to 1.0 or 100. An item with
weight 0 is unreachable. At least one item is required.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| items | [Generation.WeightedChoice.Item](#stroppy-Generation-WeightedChoice-Item) | repeated | Candidate sub-rules with their weights. At least one required. |






<a name="stroppy-Generation-WeightedChoice-Item"></a>

### Generation.WeightedChoice.Item



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rule | [Generation.Rule](#stroppy-Generation-Rule) |  | Sub-rule to dispatch to when this item is chosen. |
| weight | [double](#double) |  | Relative weight; must be &gt; 0 to be reachable. |






<a name="stroppy-OtlpExport"></a>

### OtlpExport
OtlpExport contains configuration for exporting metrics via OpenTelemetry
Protocol (OTLP). It specifies the endpoint and metrics prefix for telemetry
data export.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| otlp_grpc_endpoint | [string](#string) | optional | gRPC endpoint for OpenTelemetry collector |
| otlp_http_endpoint | [string](#string) | optional | HTTP endpoint for the OpenTelemetry collector |
| otlp_http_exporter_url_path | [string](#string) | optional | HTTP exporter path. Default is &#39;/v1/metrics&#39; |
| otlp_endpoint_insecure | [bool](#bool) | optional | Disable transport security for the exporter |
| otlp_headers | [string](#string) | optional | Headers for otlp requests e.g. Authorization=... |
| otlp_metrics_prefix | [string](#string) | optional | Prefix to be added to all exported metrics |






<a name="stroppy-Uuid"></a>

### Uuid
Uuid represents a universally unique identifier (UUID).


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| value | [string](#string) |  | String representation of UUID (e.g., &#34;123e4567-e89b-12d3-a456-426614174000&#34;) |






<a name="stroppy-Value"></a>

### Value
Value is a variant type that can represent different types of values.
It&#39;s used to represent values that can be of multiple types in a type-safe
way.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| null | [Value.NullValue](#stroppy-Value-NullValue) |  | Null value |
| int32 | [int32](#int32) |  | 32-bit signed integer |
| uint32 | [uint32](#uint32) |  | 32-bit unsigned integer |
| int64 | [int64](#int64) |  | 64-bit signed integer |
| uint64 | [uint64](#uint64) |  | 64-bit unsigned integer |
| float | [float](#float) |  | 32-bit floating point number |
| double | [double](#double) |  | 64-bit floating point number |
| string | [string](#string) |  | UTF-8 encoded string |
| bool | [bool](#bool) |  | Boolean value |
| decimal | [Decimal](#stroppy-Decimal) |  | Arbitrary-precision decimal |
| uuid | [Uuid](#stroppy-Uuid) |  | Universally unique identifier |
| datetime | [DateTime](#stroppy-DateTime) |  | Date and time |
| struct | [Value.Struct](#stroppy-Value-Struct) |  | Nested structure |
| list | [Value.List](#stroppy-Value-List) |  | List of values |
| key | [string](#string) |  | Field name (used in structs) |






<a name="stroppy-Value-List"></a>

### Value.List



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| values | [Value](#stroppy-Value) | repeated | List of values |






<a name="stroppy-Value-Struct"></a>

### Value.Struct



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fields | [Value](#stroppy-Value) | repeated | Map of field names to values |





 


<a name="stroppy-Generation-Distribution-DistributionType"></a>

### Generation.Distribution.DistributionType


| Name | Number | Description |
| ---- | ------ | ----------- |
| NORMAL | 0 | Normal (Gaussian) distribution |
| UNIFORM | 1 | Uniform distribution |
| ZIPF | 2 | Zipfian distribution |
| NURAND | 3 | TPC-C NURand(A, x, y) non-uniform distribution per spec §2.1.6: ((rand(0,A) | rand(x,y)) &#43; C) % (y - x &#43; 1) &#43; x where `|` is bitwise OR and `C` is a per-generator constant derived from the seed. The `A` parameter is carried via the `screw` field (typical TPC-C values: 255 for C_LAST, 1023 for C_ID, 8191 for OL_I_ID). Integers only — `round` must be true. |



<a name="stroppy-Generation-Distribution-NURandPhase"></a>

### Generation.Distribution.NURandPhase
For NURAND only: distinguishes C-Load vs C-Run generator instances per
TPC-C §2.1.6.1 / §5.3. The Go side derives C_load and C_run from the
same seed such that |C_run - C_load| falls within the spec&#39;s required
delta window for the active A value (255 / 1023 / 8191). Ignored by
other distribution types. Default UNSPECIFIED is treated as LOAD for
back-compat with callers that don&#39;t care about the phase.

| Name | Number | Description |
| ---- | ------ | ----------- |
| NURAND_PHASE_UNSPECIFIED | 0 | Treated as LOAD for back-compat. |
| NURAND_PHASE_LOAD | 1 | C-Load generator: used during data population. |
| NURAND_PHASE_RUN | 2 | C-Run generator: used during measurement workload. |



<a name="stroppy-Value-NullValue"></a>

### Value.NullValue


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL_VALUE | 0 | Null value |


 

 

 



<a name="proto_stroppy_config-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## proto/stroppy/config.proto



<a name="stroppy-DriverConfig"></a>

### DriverConfig
DriverConfig contains configuration for connecting to a database driver.
Driver is created as an empty shell via DriverX.create() and configured
via driver.setup(config) at runtime. Sharing semantics are determined
by the k6 lifecycle stage: init phase = shared, iteration = per-VU.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  | Database connection URL |
| driver_type | [DriverConfig.DriverType](#stroppy-DriverConfig-DriverType) |  | Name/Type of chosen driver |
| bulk_size | [int32](#int32) | optional | Rows per bulk INSERT statement (default: 500) |
| error_mode | [DriverConfig.ErrorMode](#stroppy-DriverConfig-ErrorMode) |  | How to handle errors in query/insert operations. SILENT: record metric only. LOG: record metric &#43; console.log. THROW: rethrow. FAIL: mark test for k6 as failed, continue execution, return code 110. ABORT: immediately stop test with k6 test.abort, return code 108 |
| postgres | [DriverConfig.PostgresConfig](#stroppy-DriverConfig-PostgresConfig) |  |  |
| sql | [DriverConfig.SqlConfig](#stroppy-DriverConfig-SqlConfig) |  |  |
| ca_cert_file | [string](#string) | optional | Path to CA certificate PEM file for TLS connections |
| auth_token | [string](#string) | optional | Authentication token (e.g., IAM token, API key) |
| auth_user | [string](#string) | optional | Username for static credentials auth |
| auth_password | [string](#string) | optional | Password for static credentials auth |
| tls_insecure_skip_verify | [bool](#bool) | optional | Skip TLS certificate verification (insecure, testing only) |






<a name="stroppy-DriverConfig-PostgresConfig"></a>

### DriverConfig.PostgresConfig
PostgreSQL-specific pool and connection configuration


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| trace_log_level | [string](#string) | optional | pgx trace log level: debug, info, warn, error |
| max_conn_lifetime | [string](#string) | optional | Max connection lifetime (Go duration string, e.g. &#34;1h&#34;) |
| max_conn_idle_time | [string](#string) | optional | Max connection idle time (Go duration string, e.g. &#34;10m&#34;) |
| max_conns | [int32](#int32) | optional | Maximum number of connections in the pool |
| min_conns | [int32](#int32) | optional | Minimum number of connections in the pool |
| min_idle_conns | [int32](#int32) | optional | Minimum number of idle connections |
| default_query_exec_mode | [string](#string) | optional | Query execution mode: exec, cache_statement, cache_describe, describe_exec, simple_protocol |
| description_cache_capacity | [int32](#int32) | optional | Description cache capacity (only with cache_describe mode) |
| statement_cache_capacity | [int32](#int32) | optional | Statement cache capacity (only with cache_statement mode) |






<a name="stroppy-DriverConfig-SqlConfig"></a>

### DriverConfig.SqlConfig
Generic database/sql pool settings for SQL-based drivers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| max_open_conns | [int32](#int32) | optional | Maximum number of open connections |
| max_idle_conns | [int32](#int32) | optional | Maximum number of idle connections |
| conn_max_lifetime | [string](#string) | optional | Maximum connection lifetime (Go duration string, e.g. &#34;1h&#34;) |
| conn_max_idle_time | [string](#string) | optional | Maximum idle connection time (Go duration string, e.g. &#34;10m&#34;) |






<a name="stroppy-ExporterConfig"></a>

### ExporterConfig
OtlpExporterConfig contains named configuration for an OTLP exporter.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the OTLP exporter |
| otlp_export | [OtlpExport](#stroppy-OtlpExport) |  | Configuration for the OTLP exporter |






<a name="stroppy-GlobalConfig"></a>

### GlobalConfig



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| version | [string](#string) |  | Version of the configuration format e.g. proto files version. This is used for backward compatibility of configs and will be set automatically from binary run if not present. |
| run_id | [string](#string) |  | Run identifier for reproducible test runs or debugging If set to &#34;generate()&#34; stroppy eval ulid for run_id |
| seed | [uint64](#uint64) |  | Random seed for reproducible test runs |
| metadata | [GlobalConfig.MetadataEntry](#stroppy-GlobalConfig-MetadataEntry) | repeated | Arbitrary metadata, may be passed to result labels and json output |
| logger | [LoggerConfig](#stroppy-LoggerConfig) |  | Logging configuration |
| exporter | [ExporterConfig](#stroppy-ExporterConfig) |  | Exporter configuration |






<a name="stroppy-GlobalConfig-MetadataEntry"></a>

### GlobalConfig.MetadataEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |






<a name="stroppy-LoggerConfig"></a>

### LoggerConfig
LoggerConfig contains configuration for the logging system.
It controls log levels and output formatting.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| log_level | [LoggerConfig.LogLevel](#stroppy-LoggerConfig-LogLevel) |  | Minimum log level to output |
| log_mode | [LoggerConfig.LogMode](#stroppy-LoggerConfig-LogMode) |  | Logging mode (development or production) |





 


<a name="stroppy-DriverConfig-DriverType"></a>

### DriverConfig.DriverType


| Name | Number | Description |
| ---- | ------ | ----------- |
| DRIVER_TYPE_UNSPECIFIED | 0 |  |
| DRIVER_TYPE_POSTGRES | 1 |  |
| DRIVER_TYPE_MYSQL | 2 |  |
| DRIVER_TYPE_PICODATA | 3 |  |
| DRIVER_TYPE_YDB | 4 |  |
| DRIVER_TYPE_NOOP | 5 |  |



<a name="stroppy-DriverConfig-ErrorMode"></a>

### DriverConfig.ErrorMode
Error handling mode for query and insert operations

| Name | Number | Description |
| ---- | ------ | ----------- |
| ERROR_MODE_UNSPECIFIED | 0 |  |
| ERROR_MODE_SILENT | 1 |  |
| ERROR_MODE_LOG | 2 |  |
| ERROR_MODE_THROW | 3 |  |
| ERROR_MODE_FAIL | 4 |  |
| ERROR_MODE_ABORT | 5 |  |



<a name="stroppy-LoggerConfig-LogLevel"></a>

### LoggerConfig.LogLevel


| Name | Number | Description |
| ---- | ------ | ----------- |
| LOG_LEVEL_DEBUG | 0 |  |
| LOG_LEVEL_INFO | 1 |  |
| LOG_LEVEL_WARN | 2 |  |
| LOG_LEVEL_ERROR | 3 |  |
| LOG_LEVEL_FATAL | 4 |  |



<a name="stroppy-LoggerConfig-LogMode"></a>

### LoggerConfig.LogMode


| Name | Number | Description |
| ---- | ------ | ----------- |
| LOG_MODE_DEVELOPMENT | 0 |  |
| LOG_MODE_PRODUCTION | 1 |  |


 

 

 



<a name="proto_stroppy_datagen-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## proto/stroppy/datagen.proto



<a name="stroppy-datagen-AsciiRange"></a>

### AsciiRange
AsciiRange is one contiguous [min, max] codepoint range sampled by
DrawAscii.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [uint32](#uint32) |  | Inclusive lower codepoint. |
| max | [uint32](#uint32) |  | Inclusive upper codepoint; must be &gt;= min. |






<a name="stroppy-datagen-Attr"></a>

### Attr
Attr binds a column name to the Expr that produces its value.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Column name; unique within the owning RelSource. |
| expr | [Expr](#stroppy-datagen-Expr) |  | Expression tree that produces the column value for a row. |
| null | [Null](#stroppy-datagen-Null) |  | Optional null-injection policy for this column. |






<a name="stroppy-datagen-BinOp"></a>

### BinOp
BinOp applies an arithmetic, comparison, or logical operator to sub-expressions.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| op | [BinOp.Op](#stroppy-datagen-BinOp-Op) |  | Operator to apply. |
| a | [Expr](#stroppy-datagen-Expr) |  | Left operand, or the single operand for NOT. |
| b | [Expr](#stroppy-datagen-Expr) |  | Right operand; unset for unary operators. |






<a name="stroppy-datagen-BlockRef"></a>

### BlockRef
BlockRef reads a named slot on the enclosing Side, resolved against the
current outer-side entity.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| slot | [string](#string) |  | Slot name declared on Side.block_slots. |






<a name="stroppy-datagen-BlockSlot"></a>

### BlockSlot
BlockSlot is a named expression cached per outer-side entity boundary.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Slot name; referenced by BlockRef.slot from inner-side Expr trees. |
| expr | [Expr](#stroppy-datagen-Expr) |  | Expression evaluated once per outer-side entity. |






<a name="stroppy-datagen-Call"></a>

### Call
Call invokes a stdlib function registered in pkg/datagen/stdlib.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| func | [string](#string) |  | Registered function name, e.g. &#34;std.format&#34; or &#34;std.days_to_date&#34;. |
| args | [Expr](#stroppy-datagen-Expr) | repeated | Positional arguments to the function. |






<a name="stroppy-datagen-Choose"></a>

### Choose
Choose picks one of several Expr branches at random with probability
proportional to branch weight. Only the selected branch evaluates.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| stream_id | [uint32](#uint32) |  | Compile-time assigned identifier unique within an InsertSpec; used to seed the selection draw alongside attr_path and row_index. |
| branches | [ChooseBranch](#stroppy-datagen-ChooseBranch) | repeated | Candidate branches; at least one required, all weights positive. |






<a name="stroppy-datagen-ChooseBranch"></a>

### ChooseBranch
ChooseBranch is one weighted alternative within a Choose.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| weight | [int64](#int64) |  | Positive relative weight; larger weight raises selection probability. |
| expr | [Expr](#stroppy-datagen-Expr) |  | Expression evaluated only when this branch is selected. |






<a name="stroppy-datagen-Cohort"></a>

### Cohort
Cohort is a named schedule that picks cohort_size entity IDs from
the inclusive range [entity_min, entity_max] per bucket key. The
schedule is stateless: repeated draws for the same (name, bucket_key,
slot) triple return the same entity ID across runs and workers.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Stable identifier referenced by CohortDraw.name and CohortLive.name. |
| cohort_size | [int64](#int64) |  | Number of entities drawn per active bucket; must be &lt;= span &#43; 1. |
| entity_min | [int64](#int64) |  | Inclusive lower bound on the entity ID range drawn from. |
| entity_max | [int64](#int64) |  | Inclusive upper bound on the entity ID range drawn from. |
| bucket_key | [Expr](#stroppy-datagen-Expr) |  | Default bucket-key expression; may be overridden at each call site. |
| active_every | [int64](#int64) |  | Every N-th bucket is active. 0 or 1 means every bucket is active. |
| persistence_mod | [int64](#int64) |  | Modulus used to collapse bucket keys when seeding the persistent slice. 0 disables persistence regardless of persistence_ratio. |
| persistence_ratio | [float](#float) |  | Fraction of cohort_size seeded by (bucket_key mod persistence_mod); the remainder is seeded by bucket_key directly. 0 disables persistence regardless of persistence_mod. |
| seed_salt | [uint64](#uint64) |  | Per-cohort salt providing independence across schedules that share the same entity range. |






<a name="stroppy-datagen-CohortDraw"></a>

### CohortDraw
CohortDraw reads the entity ID at position `slot` in the named
cohort&#39;s schedule for the bucket key yielded by bucket_key (falling
back to the Cohort&#39;s default bucket_key when unset).


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Cohort schedule name; must match an entry in RelSource.cohorts. |
| slot | [Expr](#stroppy-datagen-Expr) |  | Slot index within the cohort; must be in [0, cohort_size). |
| bucket_key | [Expr](#stroppy-datagen-Expr) |  | Bucket-key override; when unset the Cohort&#39;s default bucket_key is used. |






<a name="stroppy-datagen-CohortLive"></a>

### CohortLive
CohortLive reports whether the bucket named by bucket_key (or the
Cohort&#39;s default bucket_key when unset) is active in the named
cohort&#39;s schedule.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Cohort schedule name; must match an entry in RelSource.cohorts. |
| bucket_key | [Expr](#stroppy-datagen-Expr) |  | Bucket-key override; when unset the Cohort&#39;s default bucket_key is used. |






<a name="stroppy-datagen-ColRef"></a>

### ColRef
ColRef refers to another attribute in the same RelSource by name.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the referenced attribute. |






<a name="stroppy-datagen-Degree"></a>

### Degree
Degree sets how many inner rows pair with one outer row for a Side.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| fixed | [DegreeFixed](#stroppy-datagen-DegreeFixed) |  | Constant inner-row count per outer entity. |
| uniform | [DegreeUniform](#stroppy-datagen-DegreeUniform) |  | Uniform-draw inner-row count per outer entity. |






<a name="stroppy-datagen-DegreeFixed"></a>

### DegreeFixed
DegreeFixed carries a constant inner-row count per outer entity.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| count | [int64](#int64) |  | Inner rows emitted per outer-side entity. |






<a name="stroppy-datagen-DegreeUniform"></a>

### DegreeUniform
DegreeUniform draws the inner-row count from a uniform range per entity.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [int64](#int64) |  | Inclusive lower bound on inner-row count. |
| max | [int64](#int64) |  | Inclusive upper bound on inner-row count. |






<a name="stroppy-datagen-Dict"></a>

### Dict
Dict is an inline values table referenced by an opaque key in InsertSpec.dicts.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| columns | [string](#string) | repeated | Column names. Empty for scalar dicts; row values are parallel to this list. |
| weight_sets | [string](#string) | repeated | Named weight profiles. Empty list means uniform draws. Each entry names one profile — tuple-joint, per-column marginal, per-column-pair conditional — that draw operators select by name at call time. The default profile is addressed by the empty name &#34;&#34;. |
| rows | [DictRow](#stroppy-datagen-DictRow) | repeated | Row payloads. Length 1 for scalar dicts; parallel to columns otherwise. |






<a name="stroppy-datagen-DictAt"></a>

### DictAt
DictAt reads one column of one row from a Dict carried by InsertSpec.dicts.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dict_key | [string](#string) |  | Opaque dict key matching an entry in InsertSpec.dicts. |
| index | [Expr](#stroppy-datagen-Expr) |  | Row index into the dict; wrapped modulo row count at evaluation time. |
| column | [string](#string) |  | Column name for joint dicts; empty for scalar dicts. |






<a name="stroppy-datagen-DictRow"></a>

### DictRow
DictRow is one tuple of values plus optional parallel weights.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| values | [string](#string) | repeated | Column values parallel to Dict.columns (length 1 for scalar dicts). |
| weights | [int64](#int64) | repeated | Weights parallel to Dict.weight_sets. Empty when the dict is uniform. |






<a name="stroppy-datagen-DrawAscii"></a>

### DrawAscii
DrawAscii constructs a string from `alphabet` with a uniformly-drawn
length in [min_len, max_len].


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min_len | [Expr](#stroppy-datagen-Expr) |  | Inclusive lower length bound; evaluates to int64 and must be &gt;= 0. |
| max_len | [Expr](#stroppy-datagen-Expr) |  | Inclusive upper length bound; evaluates to int64 and must be &gt;= min_len. |
| alphabet | [AsciiRange](#stroppy-datagen-AsciiRange) | repeated | Codepoint ranges sampled uniformly by width. |






<a name="stroppy-datagen-DrawBernoulli"></a>

### DrawBernoulli
DrawBernoulli draws a {0, 1} int64 with probability p of 1.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| p | [float](#float) |  | Probability of a 1 outcome; must be in [0, 1]. |






<a name="stroppy-datagen-DrawDate"></a>

### DrawDate
DrawDate draws a date uniformly from an epoch-day range. Both bounds
are counted in days since 1970-01-01 UTC.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min_days_epoch | [int64](#int64) |  | Inclusive lower bound in days since the epoch. |
| max_days_epoch | [int64](#int64) |  | Inclusive upper bound in days since the epoch. |






<a name="stroppy-datagen-DrawDecimal"></a>

### DrawDecimal
DrawDecimal draws a float64 uniformly from [min, max] and rounds the
result to `scale` fractional digits.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [Expr](#stroppy-datagen-Expr) |  | Inclusive lower bound; evaluates to float64. |
| max | [Expr](#stroppy-datagen-Expr) |  | Inclusive upper bound; evaluates to float64. |
| scale | [uint32](#uint32) |  | Number of fractional digits to retain. |






<a name="stroppy-datagen-DrawDict"></a>

### DrawDict
DrawDict draws a row from a scalar Dict, optionally weighted.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dict_key | [string](#string) |  | Opaque dict key matching an entry in InsertSpec.dicts. |
| weight_set | [string](#string) |  | Weight profile to use; empty selects the default (or uniform if the dict carries no weights). |






<a name="stroppy-datagen-DrawFloatUniform"></a>

### DrawFloatUniform
DrawFloatUniform draws a float uniformly from [min, max).


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [Expr](#stroppy-datagen-Expr) |  | Inclusive lower bound; evaluates to float64. |
| max | [Expr](#stroppy-datagen-Expr) |  | Exclusive upper bound; evaluates to float64 and must be &gt; min. |






<a name="stroppy-datagen-DrawIntUniform"></a>

### DrawIntUniform
DrawIntUniform draws an integer uniformly from [min, max] inclusive.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [Expr](#stroppy-datagen-Expr) |  | Inclusive lower bound; evaluates to int64. |
| max | [Expr](#stroppy-datagen-Expr) |  | Inclusive upper bound; evaluates to int64 and must be &gt;= min. |






<a name="stroppy-datagen-DrawJoint"></a>

### DrawJoint
DrawJoint draws a tuple from a multi-column Dict and returns one
column of the chosen tuple.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| dict_key | [string](#string) |  | Opaque dict key matching an entry in InsertSpec.dicts. |
| column | [string](#string) |  | Column name whose value is returned. |
| tuple_scope | [uint32](#uint32) |  | Tuple-scoping identifier reserved for sharing one draw across several columns; D1 treats each DrawJoint as independent. |
| weight_set | [string](#string) |  | Weight profile to use; empty selects the default (or uniform). |






<a name="stroppy-datagen-DrawNURand"></a>

### DrawNURand
DrawNURand realizes the TPC-C §2.1.6 NURand(A, x, y) formula.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| a | [int64](#int64) |  | Bitmask upper bound; TPC-C spec names A. |
| x | [int64](#int64) |  | Inclusive lower bound on the output range. |
| y | [int64](#int64) |  | Inclusive upper bound on the output range. |
| c_salt | [uint64](#uint64) |  | Salt from which the per-stream constant C is derived. |






<a name="stroppy-datagen-DrawNormal"></a>

### DrawNormal
DrawNormal draws from a truncated normal clamped to [min, max].
Mean is (min&#43;max)/2 and stddev is (max-min)/(2*screw). screw=0 falls
back to the default of 3.0.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [Expr](#stroppy-datagen-Expr) |  | Inclusive lower clamp; evaluates to float64. |
| max | [Expr](#stroppy-datagen-Expr) |  | Inclusive upper clamp; evaluates to float64. |
| screw | [float](#float) |  | Screw factor; controls spread. 0 means default 3.0. |






<a name="stroppy-datagen-DrawPhrase"></a>

### DrawPhrase
DrawPhrase concatenates `n` words drawn uniformly from a vocabulary
Dict, separated by `separator`.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| vocab_key | [string](#string) |  | Opaque dict key matching an entry in InsertSpec.dicts. |
| min_words | [Expr](#stroppy-datagen-Expr) |  | Inclusive lower word-count bound; evaluates to int64 and must be &gt;= 1. |
| max_words | [Expr](#stroppy-datagen-Expr) |  | Inclusive upper word-count bound; evaluates to int64 and must be &gt;= min_words. |
| separator | [string](#string) |  | Separator joining drawn words; empty means no separator. |






<a name="stroppy-datagen-DrawZipf"></a>

### DrawZipf
DrawZipf draws from a Zipfian distribution over [min, max].


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [Expr](#stroppy-datagen-Expr) |  | Inclusive lower bound; evaluates to int64. |
| max | [Expr](#stroppy-datagen-Expr) |  | Inclusive upper bound; evaluates to int64. |
| exponent | [double](#double) |  | Skew exponent; 0 means default 1.0. |






<a name="stroppy-datagen-Expr"></a>

### Expr
Expr is the closed grammar for attribute value generation.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| col | [ColRef](#stroppy-datagen-ColRef) |  | Read another attr in the current scope by name. |
| row_index | [RowIndex](#stroppy-datagen-RowIndex) |  | Row-position indicator (entity, line, or global counter). |
| lit | [Literal](#stroppy-datagen-Literal) |  | Typed scalar constant. |
| bin_op | [BinOp](#stroppy-datagen-BinOp) |  | Binary or unary operator over sub-expressions. |
| call | [Call](#stroppy-datagen-Call) |  | Stdlib function call by registered name. |
| if_ | [If](#stroppy-datagen-If) |  | Typed ternary with lazy branch evaluation. |
| dict_at | [DictAt](#stroppy-datagen-DictAt) |  | Row lookup into a Dict carried by the owning InsertSpec. |
| block_ref | [BlockRef](#stroppy-datagen-BlockRef) |  | Named block-slot value from the enclosing Side. |
| lookup | [Lookup](#stroppy-datagen-Lookup) |  | Cross-population column read. |
| stream_draw | [StreamDraw](#stroppy-datagen-StreamDraw) |  | Seeded PRNG draw from a closed distribution catalog. |
| choose | [Choose](#stroppy-datagen-Choose) |  | Weighted random pick among Expr branches; only the selected branch evaluates. |
| cohort_draw | [CohortDraw](#stroppy-datagen-CohortDraw) |  | Entity-id draw from a named cohort schedule at a computed slot. |
| cohort_live | [CohortLive](#stroppy-datagen-CohortLive) |  | Boolean reporting whether the named cohort&#39;s bucket is active. |






<a name="stroppy-datagen-If"></a>

### If
If is a typed ternary; only the selected branch evaluates.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| cond | [Expr](#stroppy-datagen-Expr) |  | Boolean condition. |
| then | [Expr](#stroppy-datagen-Expr) |  | Expression evaluated when cond is true. |
| else_ | [Expr](#stroppy-datagen-Expr) |  | Expression evaluated when cond is false. |






<a name="stroppy-datagen-InsertSpec"></a>

### InsertSpec
InsertSpec is the boundary message a workload emits per table load.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| table | [string](#string) |  | Target table name. |
| seed | [uint64](#uint64) |  | Root PRNG seed for this load; 0 picks a random seed per run. |
| method | [InsertMethod](#stroppy-datagen-InsertMethod) |  | Wire protocol for row insertion. |
| parallelism | [Parallelism](#stroppy-datagen-Parallelism) |  | Worker hint for the Loader; clamped to the global cap. |
| source | [RelSource](#stroppy-datagen-RelSource) |  | Relational descriptor for the rows this spec emits. |
| dicts | [InsertSpec.DictsEntry](#stroppy-datagen-InsertSpec-DictsEntry) | repeated | Dict bodies keyed by the opaque TS-assigned ID that attrs reference. |






<a name="stroppy-datagen-InsertSpec-DictsEntry"></a>

### InsertSpec.DictsEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [Dict](#stroppy-datagen-Dict) |  |  |






<a name="stroppy-datagen-Literal"></a>

### Literal
Literal is a single typed scalar constant.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| int64 | [int64](#int64) |  | Signed 64-bit integer literal. |
| double | [double](#double) |  | 64-bit floating point literal. |
| string | [string](#string) |  | UTF-8 string literal. |
| bool | [bool](#bool) |  | Boolean literal. |
| bytes | [bytes](#bytes) |  | Raw bytes literal. |
| timestamp | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | Timestamp literal used for date and datetime columns. |






<a name="stroppy-datagen-Lookup"></a>

### Lookup
Lookup reads an attribute value from another population at a computed index.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target_pop | [string](#string) |  | Target population name; either the current iter-side population or an entry in the enclosing RelSource.lookup_pops. |
| attr_name | [string](#string) |  | Attribute name within the target population. |
| entity_index | [Expr](#stroppy-datagen-Expr) |  | Expression yielding the entity index within target_pop. |






<a name="stroppy-datagen-LookupPop"></a>

### LookupPop
LookupPop describes a pure sibling population that is read via Lookup only.
Its attributes are evaluated lazily and cached by the runtime.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| population | [Population](#stroppy-datagen-Population) |  | Population descriptor for the sibling; referenced by Lookup.target_pop. |
| attrs | [Attr](#stroppy-datagen-Attr) | repeated | Attribute definitions available for lookup. |
| column_order | [string](#string) | repeated | Column order for the population; parallels RelSource.column_order. |






<a name="stroppy-datagen-Null"></a>

### Null
Null carries the rate and salt that control null injection for an attr.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rate | [float](#float) |  | Probability of a null value in [0, 1]. |
| seed_salt | [uint64](#uint64) |  | Per-attr salt that keeps the null-decision stream independent from the value-generation streams. |






<a name="stroppy-datagen-Parallelism"></a>

### Parallelism
Parallelism carries worker hints from the spec author.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| workers | [int32](#int32) |  | Desired worker count; the Loader clamps to the global cap. |






<a name="stroppy-datagen-Population"></a>

### Population
Population names the entity set a RelSource iterates and its cardinality.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Stable identifier used by cross-population references. |
| size | [int64](#int64) |  | Total number of entities this population defines. |
| pure | [bool](#bool) |  | When true the population is never iterated directly; it is read through cross-population reads only. |






<a name="stroppy-datagen-RelSource"></a>

### RelSource
RelSource is the relational descriptor for the rows a spec emits.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| population | [Population](#stroppy-datagen-Population) |  | Population this spec iterates. |
| attrs | [Attr](#stroppy-datagen-Attr) | repeated | Attr definitions keyed into column_order for emission. |
| column_order | [string](#string) | repeated | Column order used when rendering rows for the driver. |
| relationships | [Relationship](#stroppy-datagen-Relationship) | repeated | Cross-population relationships this source participates in. |
| iter | [string](#string) |  | Name of the relationship in relationships that drives iteration for this source. Empty when the source iterates its own population directly. |
| cohorts | [Cohort](#stroppy-datagen-Cohort) | repeated | Named cohort schedules selecting entity slots per bucket key. |
| lookup_pops | [LookupPop](#stroppy-datagen-LookupPop) | repeated | Sibling populations referenced via Lookup but never iterated. |
| scd2 | [SCD2](#stroppy-datagen-SCD2) |  | SCD-2 row-split configuration. When set, the runtime auto-injects the named start_col / end_col values into every row based on a boundary row index: rows below boundary carry the historical pair, rows at or above carry the current pair. |






<a name="stroppy-datagen-Relationship"></a>

### Relationship
Relationship binds two or more populations into a joint iteration space.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Stable identifier; referenced by RelSource.iter. |
| sides | [Side](#stroppy-datagen-Side) | repeated | Participating sides; two or more populations project into the relation. |






<a name="stroppy-datagen-RowIndex"></a>

### RowIndex
RowIndex produces a monotonically increasing integer tied to a row position.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| kind | [RowIndex.Kind](#stroppy-datagen-RowIndex-Kind) |  | Which row counter to emit. |






<a name="stroppy-datagen-SCD2"></a>

### SCD2
SCD2 splits the population&#39;s row space into a historical slice and a
current slice at a compile-time boundary row index. The runtime
auto-injects start_col and end_col values per row; authors list these
two columns in RelSource.column_order but do not declare them in
RelSource.attrs.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| start_col | [string](#string) |  | Column name receiving the start-of-validity value. Must appear in the owning RelSource&#39;s column_order and must not be declared in column_order twice or as an attr name. |
| end_col | [string](#string) |  | Column name receiving the end-of-validity value. |
| boundary | [Expr](#stroppy-datagen-Expr) |  | Boundary row index. Rows with global row_index &lt; boundary get the historical pair; rows at or above get the current pair. The Expr must fold to a constant int64 at NewRuntime time; runtime-varying boundaries are not supported. |
| historical_start | [Expr](#stroppy-datagen-Expr) |  | Start-of-validity value for the historical slice. Evaluated once at NewRuntime against an empty-scratch context; must be constant. |
| historical_end | [Expr](#stroppy-datagen-Expr) |  | End-of-validity value for the historical slice. |
| current_start | [Expr](#stroppy-datagen-Expr) |  | Start-of-validity value for the current slice. |
| current_end | [Expr](#stroppy-datagen-Expr) |  | End-of-validity value for the current slice. When unset, the runtime emits nil (SQL NULL) for end_col on current rows. |






<a name="stroppy-datagen-Side"></a>

### Side
Side projects one population into a Relationship with a degree and strategy.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| population | [string](#string) |  | Name of the projected population; must match RelSource.population.name or a declared RelSource.lookup_pops[].population.name. |
| degree | [Degree](#stroppy-datagen-Degree) |  | How many inner entities per outer entity this side produces. |
| strategy | [Strategy](#stroppy-datagen-Strategy) |  | Pairing strategy used to map outer entities to inner ones. |
| block_slots | [BlockSlot](#stroppy-datagen-BlockSlot) | repeated | Named expressions evaluated once per outer-side entity and reused across that entity&#39;s inner rows. |






<a name="stroppy-datagen-Strategy"></a>

### Strategy
Strategy selects how outer-side entities are mapped to inner-side entities.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| hash | [StrategyHash](#stroppy-datagen-StrategyHash) |  | Hash-of-outer-index pairing. |
| sequential | [StrategySequential](#stroppy-datagen-StrategySequential) |  | Sequential walk over inner entities. |
| equitable | [StrategyEquitable](#stroppy-datagen-StrategyEquitable) |  | Equitable allocation spreading inner entities evenly across outer ones. |






<a name="stroppy-datagen-StrategyEquitable"></a>

### StrategyEquitable
StrategyEquitable distributes inner entities evenly across outer ones.






<a name="stroppy-datagen-StrategyHash"></a>

### StrategyHash
StrategyHash pairs entities by hashing the outer index.






<a name="stroppy-datagen-StrategySequential"></a>

### StrategySequential
StrategySequential walks inner entities in order.






<a name="stroppy-datagen-StreamDraw"></a>

### StreamDraw
StreamDraw carries every randomness-producing arm. stream_id is
assigned at compile time so that identical specs produce identical
streams across runs without any pointer-keyed memoization.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| stream_id | [uint32](#uint32) |  | Compile-time assigned identifier unique within an InsertSpec. The per-row PRNG is seeded from (root_seed, attr_path, stream_id, row_index); stream_id keeps multiple draws within one attr independent. |
| int_uniform | [DrawIntUniform](#stroppy-datagen-DrawIntUniform) |  | Uniform integer draw over [min, max] inclusive. |
| float_uniform | [DrawFloatUniform](#stroppy-datagen-DrawFloatUniform) |  | Uniform float draw over [min, max). |
| normal | [DrawNormal](#stroppy-datagen-DrawNormal) |  | Truncated normal draw clamped to [min, max]. |
| zipf | [DrawZipf](#stroppy-datagen-DrawZipf) |  | Zipfian power-law draw over [min, max]. |
| nurand | [DrawNURand](#stroppy-datagen-DrawNURand) |  | TPC-C §2.1.6 non-uniform random draw. |
| bernoulli | [DrawBernoulli](#stroppy-datagen-DrawBernoulli) |  | Bernoulli {0, 1} draw with probability p of 1. |
| dict | [DrawDict](#stroppy-datagen-DrawDict) |  | Weighted or uniform pick from a Dict. |
| joint | [DrawJoint](#stroppy-datagen-DrawJoint) |  | Joint tuple draw from a multi-column Dict. |
| date | [DrawDate](#stroppy-datagen-DrawDate) |  | Uniform date draw over an epoch-day range. |
| decimal | [DrawDecimal](#stroppy-datagen-DrawDecimal) |  | Uniform decimal draw rounded to a fixed scale. |
| ascii | [DrawAscii](#stroppy-datagen-DrawAscii) |  | Random ASCII string drawn from an alphabet. |
| phrase | [DrawPhrase](#stroppy-datagen-DrawPhrase) |  | Space-joined word sequence drawn from a vocabulary Dict. |





 


<a name="stroppy-datagen-BinOp-Op"></a>

### BinOp.Op
Op selects the operator; NOT is unary and uses only field `a`.

| Name | Number | Description |
| ---- | ------ | ----------- |
| OP_UNSPECIFIED | 0 |  |
| ADD | 1 | a &#43; b |
| SUB | 2 | a - b |
| MUL | 3 | a * b |
| DIV | 4 | a / b |
| MOD | 5 | a % b |
| CONCAT | 6 | String or list concatenation a || b |
| EQ | 7 | a == b |
| NE | 8 | a != b |
| LT | 9 | a &lt; b |
| LE | 10 | a &lt;= b |
| GT | 11 | a &gt; b |
| GE | 12 | a &gt;= b |
| AND | 13 | a AND b |
| OR | 14 | a OR b |
| NOT | 15 | NOT a (unary; b is ignored) |



<a name="stroppy-datagen-InsertMethod"></a>

### InsertMethod
InsertMethod selects the driver-level protocol used to write rows.

| Name | Number | Description |
| ---- | ------ | ----------- |
| PLAIN_QUERY | 0 | Parameterized SQL statement per row or batch. |
| PLAIN_BULK | 1 | Multi-row VALUES statement prepared as one query. |
| NATIVE | 2 | Driver-native path: COPY for Postgres, upload for YDB, bulk for MySQL. |



<a name="stroppy-datagen-RowIndex-Kind"></a>

### RowIndex.Kind
Kind selects which counter the index reflects.

| Name | Number | Description |
| ---- | ------ | ----------- |
| UNSPECIFIED | 0 | Default; treated as ENTITY by evaluators. |
| ENTITY | 1 | Outer iterating side in a relationship; the population&#39;s own row when no relationship is active. |
| LINE | 2 | Inner side in a relationship iteration. |
| GLOBAL | 3 | Global emitted-row counter across the whole load. |


 

 

 



<a name="proto_stroppy_descriptor-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## proto/stroppy/descriptor.proto



<a name="stroppy-InsertDescriptor"></a>

### InsertDescriptor
InsertDescription defines data to fill database.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| count | [int32](#int32) |  |  |
| table_name | [string](#string) |  | Which table to insert the values |
| method | [InsertMethod](#stroppy-InsertMethod) | optional | Allows to use a percise method of data insertion |
| seed | [uint64](#uint64) |  | Seed for data generation. 0 = random, &gt;0 = fixed (reproducible). |
| params | [QueryParamDescriptor](#stroppy-QueryParamDescriptor) | repeated | Parameters used in the insert. Names threated as db columns names, regexp is ignored. |
| groups | [QueryParamGroup](#stroppy-QueryParamGroup) | repeated | Groups of the columns |






<a name="stroppy-QueryParamDescriptor"></a>

### QueryParamDescriptor
QueryParamDescriptor defines a parameter that can be used in a query.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the parameter |
| replace_regex | [string](#string) | optional | Regular expression pattern to replace with the parameter value default is &#34;${&lt;param_name&gt;}&#34; |
| generation_rule | [Generation.Rule](#stroppy-Generation-Rule) |  | Rule for generating parameter values |
| db_specific | [Value.Struct](#stroppy-Value-Struct) | optional | Database-specific parameter properties |






<a name="stroppy-QueryParamGroup"></a>

### QueryParamGroup
QueryParamGroup defines a group of dependent parameters.
New values generated in Carthesian product manner.
It&#39;s useful to define composite primary keys.
Every evaluation step only one param changes.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Group name |
| params | [QueryParamDescriptor](#stroppy-QueryParamDescriptor) | repeated | Grouped dependent parameters |





 


<a name="stroppy-InsertMethod"></a>

### InsertMethod
Data insertion method

| Name | Number | Description |
| ---- | ------ | ----------- |
| PLAIN_QUERY | 0 |  |
| NATIVE | 1 |  |
| PLAIN_BULK | 2 |  |



<a name="stroppy-TxIsolationLevel"></a>

### TxIsolationLevel
TransactionIsolationLevel defines the isolation level for a database
transaction.

| Name | Number | Description |
| ---- | ------ | ----------- |
| UNSPECIFIED | 0 |  |
| READ_UNCOMMITTED | 1 |  |
| READ_COMMITTED | 2 |  |
| REPEATABLE_READ | 3 |  |
| SERIALIZABLE | 4 |  |
| CONNECTION_ONLY | 5 | Pinned connection without BEGIN/COMMIT. For databases without transaction support. |
| NONE | 6 | No transaction or connection pinning. Queries go through the driver pool directly. |


 

 

 



<a name="proto_stroppy_run-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## proto/stroppy/run.proto



<a name="stroppy-DriverRunConfig"></a>

### DriverRunConfig
DriverRunConfig is the user-facing driver configuration for the stroppy config file.
It mirrors the TypeScript DriverSetup interface so that protojson serialization
(camelCase field names, string values) is directly readable by declareDriverSetup()
in the TypeScript layer via the STROPPY_DRIVER_N environment variable.

This is intentionally separate from DriverConfig (the runtime binary proto for TS-&gt;Go dispatch).


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| driver_type | [string](#string) |  | Driver type. One of: &#34;postgres&#34;, &#34;mysql&#34;, &#34;picodata&#34;, &#34;ydb&#34;, &#34;noop&#34;. Matches TS DriverSetup.driverType (string union, not proto enum). |
| url | [string](#string) |  | Database connection URL |
| default_insert_method | [string](#string) |  | Default insert method. One of: &#34;native&#34;, &#34;plain_bulk&#34;, &#34;plain_query&#34;. Matches TS DriverSetup.defaultInsertMethod. |
| pool | [DriverRunConfig.PoolConfig](#stroppy-DriverRunConfig-PoolConfig) | optional |  |
| error_mode | [string](#string) |  | Error handling mode. One of: &#34;silent&#34;, &#34;log&#34;, &#34;throw&#34;, &#34;fail&#34;, &#34;abort&#34;. Matches TS DriverSetup.errorMode. |
| bulk_size | [int32](#int32) | optional | Rows per bulk INSERT statement. Matches TS DriverSetup.bulkSize. |
| ca_cert_file | [string](#string) | optional | Path to CA certificate PEM file. Matches TS DriverSetup.caCertFile. |
| auth_token | [string](#string) | optional | Authentication token (e.g. IAM token). Matches TS DriverSetup.authToken. |
| auth_user | [string](#string) | optional | Username for static credentials. Matches TS DriverSetup.authUser. |
| auth_password | [string](#string) | optional | Password for static credentials. Matches TS DriverSetup.authPassword. |
| tls_insecure_skip_verify | [bool](#bool) | optional | Skip TLS certificate verification. Matches TS DriverSetup.tlsInsecureSkipVerify. |
| default_tx_isolation | [string](#string) |  | Default transaction isolation level. One of: &#34;read_uncommitted&#34;, &#34;read_committed&#34;, &#34;repeatable_read&#34;, &#34;serializable&#34;. Matches TS DriverSetup.defaultTxIsolation. |






<a name="stroppy-DriverRunConfig-PoolConfig"></a>

### DriverRunConfig.PoolConfig
Pool configuration. Sugar field that maps to PostgresConfig or SqlConfig
in the TypeScript layer based on driver_type.
Matches TS DriverSetup.pool (PoolConfig).


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| max_conns | [int32](#int32) | optional | pgx / postgres pool fields |
| min_conns | [int32](#int32) | optional |  |
| min_idle_conns | [int32](#int32) | optional |  |
| max_conn_lifetime | [string](#string) | optional |  |
| max_conn_idle_time | [string](#string) | optional |  |
| trace_log_level | [string](#string) | optional |  |
| default_query_exec_mode | [string](#string) | optional |  |
| description_cache_capacity | [int32](#int32) | optional |  |
| statement_cache_capacity | [int32](#int32) | optional |  |
| max_open_conns | [int32](#int32) | optional | database/sql pool fields (mysql, ydb) |
| max_idle_conns | [int32](#int32) | optional |  |
| conn_max_lifetime | [string](#string) | optional |  |
| conn_max_idle_time | [string](#string) | optional |  |






<a name="stroppy-RunConfig"></a>

### RunConfig
RunConfig is the top-level stroppy config file schema.

Default file: ./stroppy-config.json (auto-discovered by stroppy run and stroppy probe).
Override with -f/--file flag.

Precedence (highest to lowest):
  real env &gt; -e flags &gt; this file &gt; -d/-D presets &gt; script defaults

Example (stroppy-config.json):
  {
    &#34;version&#34;: &#34;1&#34;,
    &#34;script&#34;: &#34;tpcc&#34;,
    &#34;global&#34;: {
      &#34;logger&#34;: { &#34;logLevel&#34;: &#34;LOG_LEVEL_INFO&#34; }
    },
    &#34;drivers&#34;: {
      &#34;0&#34;: { &#34;driverType&#34;: &#34;postgres&#34;, &#34;url&#34;: &#34;postgres://user:pass@db:5432/bench&#34;,
              &#34;pool&#34;: { &#34;maxConns&#34;: 200 } }
    },
    &#34;env&#34;: { &#34;WAREHOUSES&#34;: &#34;10&#34; },
    &#34;k6Args&#34;: [&#34;--vus&#34;, &#34;10&#34;, &#34;--duration&#34;, &#34;30m&#34;]
  }


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| version | [string](#string) |  | Config file format version. Currently &#34;1&#34;. |
| script | [string](#string) | optional | Script to run: preset name (e.g. &#34;tpcc&#34;), .ts path, .sql path, or inline SQL. If omitted, the first CLI positional argument is used. |
| sql | [string](#string) | optional | SQL file: path or name. If omitted, auto-resolved from script/preset. |
| global | [GlobalConfig](#stroppy-GlobalConfig) |  | Global stroppy settings: logger level/mode, OTEL exporter, seed, run_id, metadata. These activate the previously-dead GlobalConfig plumbing. |
| drivers | [RunConfig.DriversEntry](#stroppy-RunConfig-DriversEntry) | repeated | Driver configurations indexed by driver number (0-based string key in JSON). Equivalent to -d/-D CLI flags but type-safe and with full pool config support. CLI -d/-D takes precedence over entries here. |
| env | [RunConfig.EnvEntry](#stroppy-RunConfig-EnvEntry) | repeated | Environment variable overrides for the k6 script. Equivalent to -e KEY=VALUE. All keys are uppercased on load. Precedence: real env &gt; -e flags &gt; this map &gt; script ENV() defaults. |
| steps | [string](#string) | repeated | Step allowlist. Equivalent to --steps. CLI --steps takes precedence; this is used only when --steps is absent. |
| no_steps | [string](#string) | repeated | Step blocklist. Equivalent to --no-steps. CLI --no-steps takes precedence; this is used only when --no-steps is absent. |
| k6_args | [string](#string) | repeated | Additional raw args passed directly to &#34;k6 run&#34;. Placed before CLI -- args, so CLI args can override (last-wins for most k6 flags). Example: [&#34;--vus&#34;, &#34;10&#34;, &#34;--duration&#34;, &#34;30m&#34;] |
| k6_config | [string](#string) | optional | Optional path to a k6 native config JSON file. If set, &#34;--config &lt;path&gt;&#34; is prepended to k6 run args. Useful for setting scenarios, thresholds, or other options not available via CLI flags. |






<a name="stroppy-RunConfig-DriversEntry"></a>

### RunConfig.DriversEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [uint32](#uint32) |  |  |
| value | [DriverRunConfig](#stroppy-DriverRunConfig) |  |  |






<a name="stroppy-RunConfig-EnvEntry"></a>

### RunConfig.EnvEntry



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| key | [string](#string) |  |  |
| value | [string](#string) |  |  |





 

 

 

 



<a name="proto_stroppy_runtime-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## proto/stroppy/runtime.proto



<a name="stroppy-DriverQuery"></a>

### DriverQuery
DriverQuery represents a query that can be executed by a database driver.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| request | [string](#string) |  | Request of the query |
| params | [Value](#stroppy-Value) | repeated | Parameters of the query |
| method | [InsertMethod](#stroppy-InsertMethod) | optional | If alternate insertion method required |






<a name="stroppy-DriverQueryStat"></a>

### DriverQueryStat
DriverQueryStat represent an actual time spent on single query.
exec_duration includes the network round-trip and exection on dbms.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| exec_duration | [google.protobuf.Duration](#google-protobuf-Duration) |  |  |






<a name="stroppy-DriverTransaction"></a>

### DriverTransaction
DriverTransaction represents a transaction that can be executed by a database
driver.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| queries | [DriverQuery](#stroppy-DriverQuery) | repeated | Queries of the transaction |
| isolation_level | [TxIsolationLevel](#stroppy-TxIsolationLevel) |  | Isolation level of the transaction |






<a name="stroppy-DriverTransactionStat"></a>

### DriverTransactionStat
DriverTransactionStat represents an actual time spent on transaction.
exec_duration includes the network round-trip and exection on dbms.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| queries | [DriverQueryStat](#stroppy-DriverQueryStat) | repeated |  |
| exec_duration | [google.protobuf.Duration](#google-protobuf-Duration) |  |  |
| isolation_level | [TxIsolationLevel](#stroppy-TxIsolationLevel) |  |  |





 

 

 

 



## Scalar Value Types

| .proto Type | Notes | C++ | Java | Python | Go | C# | PHP | Ruby |
| ----------- | ----- | --- | ---- | ------ | -- | -- | --- | ---- |
| <a name="double" /> double |  | double | double | float | float64 | double | float | Float |
| <a name="float" /> float |  | float | float | float | float32 | float | float | Float |
| <a name="int32" /> int32 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32 instead. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="int64" /> int64 | Uses variable-length encoding. Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint64 instead. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="uint32" /> uint32 | Uses variable-length encoding. | uint32 | int | int/long | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="uint64" /> uint64 | Uses variable-length encoding. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum or Fixnum (as required) |
| <a name="sint32" /> sint32 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int32s. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sint64" /> sint64 | Uses variable-length encoding. Signed int value. These more efficiently encode negative numbers than regular int64s. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="fixed32" /> fixed32 | Always four bytes. More efficient than uint32 if values are often greater than 2^28. | uint32 | int | int | uint32 | uint | integer | Bignum or Fixnum (as required) |
| <a name="fixed64" /> fixed64 | Always eight bytes. More efficient than uint64 if values are often greater than 2^56. | uint64 | long | int/long | uint64 | ulong | integer/string | Bignum |
| <a name="sfixed32" /> sfixed32 | Always four bytes. | int32 | int | int | int32 | int | integer | Bignum or Fixnum (as required) |
| <a name="sfixed64" /> sfixed64 | Always eight bytes. | int64 | long | int/long | int64 | long | integer/string | Bignum |
| <a name="bool" /> bool |  | bool | boolean | boolean | bool | bool | boolean | TrueClass/FalseClass |
| <a name="string" /> string | A string must always contain UTF-8 encoded or 7-bit ASCII text. | string | String | str/unicode | string | string | string | String (UTF-8) |
| <a name="bytes" /> bytes | May contain any arbitrary sequence of bytes. | string | ByteString | str | []byte | ByteString | string | String (ASCII-8BIT) |

