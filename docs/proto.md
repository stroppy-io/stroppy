# Protocol Documentation
<a name="top"></a>

## Table of Contents

- [gen/defaults/defaults.proto](#gen_defaults_defaults-proto)
    - [FieldDefaults](#defaults-FieldDefaults)
    - [MessageDefaults](#defaults-MessageDefaults)
  
    - [File-level Extensions](#gen_defaults_defaults-proto-extensions)
    - [File-level Extensions](#gen_defaults_defaults-proto-extensions)
    - [File-level Extensions](#gen_defaults_defaults-proto-extensions)
    - [File-level Extensions](#gen_defaults_defaults-proto-extensions)
    - [File-level Extensions](#gen_defaults_defaults-proto-extensions)
  
- [gen/validate/validate.proto](#gen_validate_validate-proto)
    - [BoolRules](#validate-BoolRules)
    - [BytesRules](#validate-BytesRules)
    - [DoubleRules](#validate-DoubleRules)
    - [EnumRules](#validate-EnumRules)
    - [FieldRules](#validate-FieldRules)
    - [Fixed32Rules](#validate-Fixed32Rules)
    - [Fixed64Rules](#validate-Fixed64Rules)
    - [FloatRules](#validate-FloatRules)
    - [Int32Rules](#validate-Int32Rules)
    - [Int64Rules](#validate-Int64Rules)
    - [MapRules](#validate-MapRules)
    - [MessageRules](#validate-MessageRules)
    - [RepeatedRules](#validate-RepeatedRules)
    - [SFixed32Rules](#validate-SFixed32Rules)
    - [SFixed64Rules](#validate-SFixed64Rules)
    - [SInt32Rules](#validate-SInt32Rules)
    - [SInt64Rules](#validate-SInt64Rules)
    - [StringRules](#validate-StringRules)
    - [UInt32Rules](#validate-UInt32Rules)
    - [UInt64Rules](#validate-UInt64Rules)
  
    - [KnownRegex](#validate-KnownRegex)
  
    - [File-level Extensions](#gen_validate_validate-proto-extensions)
    - [File-level Extensions](#gen_validate_validate-proto-extensions)
    - [File-level Extensions](#gen_validate_validate-proto-extensions)
    - [File-level Extensions](#gen_validate_validate-proto-extensions)
  
- [config.proto](#config-proto)
    - [CloudConfig](#stroppy-CloudConfig)
    - [ConfigFile](#stroppy-ConfigFile)
    - [DriverConfig](#stroppy-DriverConfig)
    - [ExecutorConfig](#stroppy-ExecutorConfig)
    - [ExporterConfig](#stroppy-ExporterConfig)
    - [GlobalConfig](#stroppy-GlobalConfig)
    - [GlobalConfig.MetadataEntry](#stroppy-GlobalConfig-MetadataEntry)
    - [LoggerConfig](#stroppy-LoggerConfig)
    - [SideCarConfig](#stroppy-SideCarConfig)
    - [Step](#stroppy-Step)
  
    - [DriverConfig.DriverType](#stroppy-DriverConfig-DriverType)
    - [LoggerConfig.LogLevel](#stroppy-LoggerConfig-LogLevel)
    - [LoggerConfig.LogMode](#stroppy-LoggerConfig-LogMode)
  
- [k6.proto](#k6-proto)
    - [ConstantArrivalRate](#stroppy-ConstantArrivalRate)
    - [ConstantVUs](#stroppy-ConstantVUs)
    - [K6Options](#stroppy-K6Options)
    - [K6Scenario](#stroppy-K6Scenario)
    - [PerVuIterations](#stroppy-PerVuIterations)
    - [RampingArrivalRate](#stroppy-RampingArrivalRate)
    - [RampingArrivalRate.RateStage](#stroppy-RampingArrivalRate-RateStage)
    - [RampingVUs](#stroppy-RampingVUs)
    - [RampingVUs.VUStage](#stroppy-RampingVUs-VUStage)
    - [SharedIterations](#stroppy-SharedIterations)
  
- [runtime.proto](#runtime-proto)
    - [DriverQuery](#stroppy-DriverQuery)
    - [DriverQueryStat](#stroppy-DriverQueryStat)
    - [DriverTransaction](#stroppy-DriverTransaction)
    - [DriverTransactionStat](#stroppy-DriverTransactionStat)
    - [StepContext](#stroppy-StepContext)
    - [UnitContext](#stroppy-UnitContext)
  
- [sidecar.proto](#sidecar-proto)
    - [SidecarService](#stroppy-SidecarService)
  
- [common.proto](#common-proto)
    - [DateTime](#stroppy-DateTime)
    - [Decimal](#stroppy-Decimal)
    - [Generation](#stroppy-Generation)
    - [Generation.Alphabet](#stroppy-Generation-Alphabet)
    - [Generation.Distribution](#stroppy-Generation-Distribution)
    - [Generation.Range](#stroppy-Generation-Range)
    - [Generation.Range.AnyStringRange](#stroppy-Generation-Range-AnyStringRange)
    - [Generation.Range.DateTimeRange](#stroppy-Generation-Range-DateTimeRange)
    - [Generation.Range.DateTimeRange.Default](#stroppy-Generation-Range-DateTimeRange-Default)
    - [Generation.Range.DateTimeRange.Timestamp](#stroppy-Generation-Range-DateTimeRange-Timestamp)
    - [Generation.Range.DateTimeRange.TimestampPb](#stroppy-Generation-Range-DateTimeRange-TimestampPb)
    - [Generation.Range.DecimalRange](#stroppy-Generation-Range-DecimalRange)
    - [Generation.Range.DecimalRange.Default](#stroppy-Generation-Range-DecimalRange-Default)
    - [Generation.Range.DoubleRange](#stroppy-Generation-Range-DoubleRange)
    - [Generation.Range.FloatRange](#stroppy-Generation-Range-FloatRange)
    - [Generation.Range.Int32Range](#stroppy-Generation-Range-Int32Range)
    - [Generation.Range.Int64Range](#stroppy-Generation-Range-Int64Range)
    - [Generation.Range.UInt32Range](#stroppy-Generation-Range-UInt32Range)
    - [Generation.Range.UInt64Range](#stroppy-Generation-Range-UInt64Range)
    - [Generation.Rule](#stroppy-Generation-Rule)
    - [Generation.Rules](#stroppy-Generation-Rules)
    - [Generation.Rules.BoolRule](#stroppy-Generation-Rules-BoolRule)
    - [Generation.Rules.DateTimeRule](#stroppy-Generation-Rules-DateTimeRule)
    - [Generation.Rules.DecimalRule](#stroppy-Generation-Rules-DecimalRule)
    - [Generation.Rules.DoubleRule](#stroppy-Generation-Rules-DoubleRule)
    - [Generation.Rules.FloatRule](#stroppy-Generation-Rules-FloatRule)
    - [Generation.Rules.Int32Rule](#stroppy-Generation-Rules-Int32Rule)
    - [Generation.Rules.Int64Rule](#stroppy-Generation-Rules-Int64Rule)
    - [Generation.Rules.StringRule](#stroppy-Generation-Rules-StringRule)
    - [Generation.Rules.UInt32Rule](#stroppy-Generation-Rules-UInt32Rule)
    - [Generation.Rules.UInt64Rule](#stroppy-Generation-Rules-UInt64Rule)
    - [Generation.Rules.UuidRule](#stroppy-Generation-Rules-UuidRule)
    - [OtlpExport](#stroppy-OtlpExport)
    - [Uuid](#stroppy-Uuid)
    - [Value](#stroppy-Value)
    - [Value.List](#stroppy-Value-List)
    - [Value.Struct](#stroppy-Value-Struct)
  
    - [Generation.Distribution.DistributionType](#stroppy-Generation-Distribution-DistributionType)
    - [Value.NullValue](#stroppy-Value-NullValue)
  
- [descriptor.proto](#descriptor-proto)
    - [BenchmarkDescriptor](#stroppy-BenchmarkDescriptor)
    - [ColumnDescriptor](#stroppy-ColumnDescriptor)
    - [IndexDescriptor](#stroppy-IndexDescriptor)
    - [QueryDescriptor](#stroppy-QueryDescriptor)
    - [QueryParamDescriptor](#stroppy-QueryParamDescriptor)
    - [QueryParamGroup](#stroppy-QueryParamGroup)
    - [TableDescriptor](#stroppy-TableDescriptor)
    - [TransactionDescriptor](#stroppy-TransactionDescriptor)
    - [UnitDescriptor](#stroppy-UnitDescriptor)
    - [WorkloadDescriptor](#stroppy-WorkloadDescriptor)
    - [WorkloadUnitDescriptor](#stroppy-WorkloadUnitDescriptor)
  
    - [TxIsolationLevel](#stroppy-TxIsolationLevel)
  
- [Scalar Value Types](#scalar-value-types)



<a name="gen_defaults_defaults-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## gen/defaults/defaults.proto



<a name="defaults-FieldDefaults"></a>

### FieldDefaults
FieldDefaults encapsulates the default values for each type of field. Depending on the
field, the correct set should be used to ensure proper defaults generation.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| float | [float](#float) | optional | Scalar Field Types |
| double | [double](#double) | optional |  |
| int32 | [int32](#int32) | optional |  |
| int64 | [int64](#int64) | optional |  |
| uint32 | [uint32](#uint32) | optional |  |
| uint64 | [uint64](#uint64) | optional |  |
| sint32 | [sint32](#sint32) | optional |  |
| sint64 | [sint64](#sint64) | optional |  |
| fixed32 | [fixed32](#fixed32) | optional |  |
| fixed64 | [fixed64](#fixed64) | optional |  |
| sfixed32 | [sfixed32](#sfixed32) | optional |  |
| sfixed64 | [sfixed64](#sfixed64) | optional |  |
| bool | [bool](#bool) | optional |  |
| string | [string](#string) | optional |  |
| bytes | [bytes](#bytes) | optional |  |
| enum | [uint32](#uint32) | optional | Complex Field Types |
| message | [MessageDefaults](#defaults-MessageDefaults) | optional | repeated = 18; map = 19; |
| duration | [string](#string) | optional | Well-Known Field Types any = 20; |
| timestamp | [string](#string) | optional |  |






<a name="defaults-MessageDefaults"></a>

### MessageDefaults
MessageDefaults define the default behaviour for this field.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| initialize | [bool](#bool) | optional | Initialize specify that the message should be initialized |
| defaults | [bool](#bool) | optional | Defaults specifies that the messages&#39; defaults should be applied |





 

 


<a name="gen_defaults_defaults-proto-extensions"></a>

### File-level Extensions
| Extension | Type | Base | Number | Description |
| --------- | ---- | ---- | ------ | ----------- |
| value | FieldDefaults | .google.protobuf.FieldOptions | 1171 | Value specify the default value to set on this field. By default, none is set on a field. |
| disabled | bool | .google.protobuf.MessageOptions | 1171 | Disabled nullifies any defaults for this message, including any message fields associated with it that do support defaults. |
| ignored | bool | .google.protobuf.MessageOptions | 1172 | Ignore skips generation of default methods for this message. |
| unexported | bool | .google.protobuf.MessageOptions | 1173 | Unexported generate an unexported defaults method, this can be useful when we want both the generated defaults and a custom defaults method that will call the unexported method. |
| oneof | string | .google.protobuf.OneofOptions | 1171 |  |

 

 



<a name="gen_validate_validate-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## gen/validate/validate.proto



<a name="validate-BoolRules"></a>

### BoolRules
BoolRules describes the constraints applied to `bool` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [bool](#bool) | optional | Const specifies that this field must be exactly the specified value |






<a name="validate-BytesRules"></a>

### BytesRules
BytesRules describe the constraints applied to `bytes` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [bytes](#bytes) | optional | Const specifies that this field must be exactly the specified value |
| len | [uint64](#uint64) | optional | Len specifies that this field must be the specified number of bytes |
| min_len | [uint64](#uint64) | optional | MinLen specifies that this field must be the specified number of bytes at a minimum |
| max_len | [uint64](#uint64) | optional | MaxLen specifies that this field must be the specified number of bytes at a maximum |
| pattern | [string](#string) | optional | Pattern specifies that this field must match against the specified regular expression (RE2 syntax). The included expression should elide any delimiters. |
| prefix | [bytes](#bytes) | optional | Prefix specifies that this field must have the specified bytes at the beginning of the string. |
| suffix | [bytes](#bytes) | optional | Suffix specifies that this field must have the specified bytes at the end of the string. |
| contains | [bytes](#bytes) | optional | Contains specifies that this field must have the specified bytes anywhere in the string. |
| in | [bytes](#bytes) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [bytes](#bytes) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ip | [bool](#bool) | optional | Ip specifies that the field must be a valid IP (v4 or v6) address in byte format |
| ipv4 | [bool](#bool) | optional | Ipv4 specifies that the field must be a valid IPv4 address in byte format |
| ipv6 | [bool](#bool) | optional | Ipv6 specifies that the field must be a valid IPv6 address in byte format |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-DoubleRules"></a>

### DoubleRules
DoubleRules describes the constraints applied to `double` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [double](#double) | optional | Const specifies that this field must be exactly the specified value |
| lt | [double](#double) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [double](#double) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [double](#double) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [double](#double) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [double](#double) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [double](#double) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-EnumRules"></a>

### EnumRules
EnumRules describe the constraints applied to enum values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [int32](#int32) | optional | Const specifies that this field must be exactly the specified value |
| defined_only | [bool](#bool) | optional | DefinedOnly specifies that this field must be only one of the defined values for this enum, failing on any undefined value. |
| in | [int32](#int32) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [int32](#int32) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |






<a name="validate-FieldRules"></a>

### FieldRules
FieldRules encapsulates the rules for each type of field. Depending on the
field, the correct set should be used to ensure proper validations.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| message | [MessageRules](#validate-MessageRules) | optional |  |
| float | [FloatRules](#validate-FloatRules) | optional | Scalar Field Types |
| double | [DoubleRules](#validate-DoubleRules) | optional |  |
| int32 | [Int32Rules](#validate-Int32Rules) | optional |  |
| int64 | [Int64Rules](#validate-Int64Rules) | optional |  |
| uint32 | [UInt32Rules](#validate-UInt32Rules) | optional |  |
| uint64 | [UInt64Rules](#validate-UInt64Rules) | optional |  |
| sint32 | [SInt32Rules](#validate-SInt32Rules) | optional |  |
| sint64 | [SInt64Rules](#validate-SInt64Rules) | optional |  |
| fixed32 | [Fixed32Rules](#validate-Fixed32Rules) | optional |  |
| fixed64 | [Fixed64Rules](#validate-Fixed64Rules) | optional |  |
| sfixed32 | [SFixed32Rules](#validate-SFixed32Rules) | optional |  |
| sfixed64 | [SFixed64Rules](#validate-SFixed64Rules) | optional |  |
| bool | [BoolRules](#validate-BoolRules) | optional |  |
| string | [StringRules](#validate-StringRules) | optional |  |
| bytes | [BytesRules](#validate-BytesRules) | optional |  |
| enum | [EnumRules](#validate-EnumRules) | optional | Complex Field Types |
| repeated | [RepeatedRules](#validate-RepeatedRules) | optional |  |
| map | [MapRules](#validate-MapRules) | optional |  |






<a name="validate-Fixed32Rules"></a>

### Fixed32Rules
Fixed32Rules describes the constraints applied to `fixed32` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [fixed32](#fixed32) | optional | Const specifies that this field must be exactly the specified value |
| lt | [fixed32](#fixed32) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [fixed32](#fixed32) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [fixed32](#fixed32) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [fixed32](#fixed32) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [fixed32](#fixed32) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [fixed32](#fixed32) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-Fixed64Rules"></a>

### Fixed64Rules
Fixed64Rules describes the constraints applied to `fixed64` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [fixed64](#fixed64) | optional | Const specifies that this field must be exactly the specified value |
| lt | [fixed64](#fixed64) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [fixed64](#fixed64) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [fixed64](#fixed64) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [fixed64](#fixed64) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [fixed64](#fixed64) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [fixed64](#fixed64) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-FloatRules"></a>

### FloatRules
FloatRules describes the constraints applied to `float` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [float](#float) | optional | Const specifies that this field must be exactly the specified value |
| lt | [float](#float) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [float](#float) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [float](#float) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [float](#float) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [float](#float) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [float](#float) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-Int32Rules"></a>

### Int32Rules
Int32Rules describes the constraints applied to `int32` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [int32](#int32) | optional | Const specifies that this field must be exactly the specified value |
| lt | [int32](#int32) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [int32](#int32) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [int32](#int32) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [int32](#int32) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [int32](#int32) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [int32](#int32) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-Int64Rules"></a>

### Int64Rules
Int64Rules describes the constraints applied to `int64` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [int64](#int64) | optional | Const specifies that this field must be exactly the specified value |
| lt | [int64](#int64) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [int64](#int64) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [int64](#int64) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [int64](#int64) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [int64](#int64) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [int64](#int64) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-MapRules"></a>

### MapRules
MapRules describe the constraints applied to `map` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min_pairs | [uint64](#uint64) | optional | MinPairs specifies that this field must have the specified number of KVs at a minimum |
| max_pairs | [uint64](#uint64) | optional | MaxPairs specifies that this field must have the specified number of KVs at a maximum |
| no_sparse | [bool](#bool) | optional | NoSparse specifies values in this field cannot be unset. This only applies to map&#39;s with message value types. |
| keys | [FieldRules](#validate-FieldRules) | optional | Keys specifies the constraints to be applied to each key in the field. |
| values | [FieldRules](#validate-FieldRules) | optional | Values specifies the constraints to be applied to the value of each key in the field. Message values will still have their validations evaluated unless skip is specified here. |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-MessageRules"></a>

### MessageRules
MessageRules describe the constraints applied to embedded message values.
For message-type fields, validation is performed recursively.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| skip | [bool](#bool) | optional | Skip specifies that the validation rules of this field should not be evaluated |
| required | [bool](#bool) | optional | Required specifies that this field must be set |






<a name="validate-RepeatedRules"></a>

### RepeatedRules
RepeatedRules describe the constraints applied to `repeated` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min_items | [uint64](#uint64) | optional | MinItems specifies that this field must have the specified number of items at a minimum |
| max_items | [uint64](#uint64) | optional | MaxItems specifies that this field must have the specified number of items at a maximum |
| unique | [bool](#bool) | optional | Unique specifies that all elements in this field must be unique. This constraint is only applicable to scalar and enum types (messages are not supported). |
| items | [FieldRules](#validate-FieldRules) | optional | Items specifies the constraints to be applied to each item in the field. Repeated message fields will still execute validation against each item unless skip is specified here. |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-SFixed32Rules"></a>

### SFixed32Rules
SFixed32Rules describes the constraints applied to `sfixed32` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [sfixed32](#sfixed32) | optional | Const specifies that this field must be exactly the specified value |
| lt | [sfixed32](#sfixed32) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [sfixed32](#sfixed32) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [sfixed32](#sfixed32) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [sfixed32](#sfixed32) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [sfixed32](#sfixed32) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [sfixed32](#sfixed32) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-SFixed64Rules"></a>

### SFixed64Rules
SFixed64Rules describes the constraints applied to `sfixed64` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [sfixed64](#sfixed64) | optional | Const specifies that this field must be exactly the specified value |
| lt | [sfixed64](#sfixed64) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [sfixed64](#sfixed64) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [sfixed64](#sfixed64) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [sfixed64](#sfixed64) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [sfixed64](#sfixed64) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [sfixed64](#sfixed64) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-SInt32Rules"></a>

### SInt32Rules
SInt32Rules describes the constraints applied to `sint32` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [sint32](#sint32) | optional | Const specifies that this field must be exactly the specified value |
| lt | [sint32](#sint32) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [sint32](#sint32) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [sint32](#sint32) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [sint32](#sint32) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [sint32](#sint32) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [sint32](#sint32) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-SInt64Rules"></a>

### SInt64Rules
SInt64Rules describes the constraints applied to `sint64` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [sint64](#sint64) | optional | Const specifies that this field must be exactly the specified value |
| lt | [sint64](#sint64) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [sint64](#sint64) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [sint64](#sint64) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [sint64](#sint64) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [sint64](#sint64) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [sint64](#sint64) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-StringRules"></a>

### StringRules
StringRules describe the constraints applied to `string` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [string](#string) | optional | Const specifies that this field must be exactly the specified value |
| len | [uint64](#uint64) | optional | Len specifies that this field must be the specified number of characters (Unicode code points). Note that the number of characters may differ from the number of bytes in the string. |
| min_len | [uint64](#uint64) | optional | MinLen specifies that this field must be the specified number of characters (Unicode code points) at a minimum. Note that the number of characters may differ from the number of bytes in the string. |
| max_len | [uint64](#uint64) | optional | MaxLen specifies that this field must be the specified number of characters (Unicode code points) at a maximum. Note that the number of characters may differ from the number of bytes in the string. |
| len_bytes | [uint64](#uint64) | optional | LenBytes specifies that this field must be the specified number of bytes |
| min_bytes | [uint64](#uint64) | optional | MinBytes specifies that this field must be the specified number of bytes at a minimum |
| max_bytes | [uint64](#uint64) | optional | MaxBytes specifies that this field must be the specified number of bytes at a maximum |
| pattern | [string](#string) | optional | Pattern specifies that this field must match against the specified regular expression (RE2 syntax). The included expression should elide any delimiters. |
| prefix | [string](#string) | optional | Prefix specifies that this field must have the specified substring at the beginning of the string. |
| suffix | [string](#string) | optional | Suffix specifies that this field must have the specified substring at the end of the string. |
| contains | [string](#string) | optional | Contains specifies that this field must have the specified substring anywhere in the string. |
| not_contains | [string](#string) | optional | NotContains specifies that this field cannot have the specified substring anywhere in the string. |
| in | [string](#string) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [string](#string) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| email | [bool](#bool) | optional | Email specifies that the field must be a valid email address as defined by RFC 5322 |
| hostname | [bool](#bool) | optional | Hostname specifies that the field must be a valid hostname as defined by RFC 1034. This constraint does not support internationalized domain names (IDNs). |
| ip | [bool](#bool) | optional | Ip specifies that the field must be a valid IP (v4 or v6) address. Valid IPv6 addresses should not include surrounding square brackets. |
| ipv4 | [bool](#bool) | optional | Ipv4 specifies that the field must be a valid IPv4 address. |
| ipv6 | [bool](#bool) | optional | Ipv6 specifies that the field must be a valid IPv6 address. Valid IPv6 addresses should not include surrounding square brackets. |
| uri | [bool](#bool) | optional | Uri specifies that the field must be a valid, absolute URI as defined by RFC 3986 |
| uri_ref | [bool](#bool) | optional | UriRef specifies that the field must be a valid URI as defined by RFC 3986 and may be relative or absolute. |
| address | [bool](#bool) | optional | Address specifies that the field must be either a valid hostname as defined by RFC 1034 (which does not support internationalized domain names or IDNs), or it can be a valid IP (v4 or v6). |
| uuid | [bool](#bool) | optional | Uuid specifies that the field must be a valid UUID as defined by RFC 4122 |
| well_known_regex | [KnownRegex](#validate-KnownRegex) | optional | WellKnownRegex specifies a common well known pattern defined as a regex. |
| strict | [bool](#bool) | optional | This applies to regexes HTTP_HEADER_NAME and HTTP_HEADER_VALUE to enable strict header validation. By default, this is true, and HTTP header validations are RFC-compliant. Setting to false will enable a looser validations that only disallows \r\n\0 characters, which can be used to bypass header matching rules. Default: true |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-UInt32Rules"></a>

### UInt32Rules
UInt32Rules describes the constraints applied to `uint32` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [uint32](#uint32) | optional | Const specifies that this field must be exactly the specified value |
| lt | [uint32](#uint32) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [uint32](#uint32) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [uint32](#uint32) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [uint32](#uint32) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [uint32](#uint32) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [uint32](#uint32) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |






<a name="validate-UInt64Rules"></a>

### UInt64Rules
UInt64Rules describes the constraints applied to `uint64` values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| const | [uint64](#uint64) | optional | Const specifies that this field must be exactly the specified value |
| lt | [uint64](#uint64) | optional | Lt specifies that this field must be less than the specified value, exclusive |
| lte | [uint64](#uint64) | optional | Lte specifies that this field must be less than or equal to the specified value, inclusive |
| gt | [uint64](#uint64) | optional | Gt specifies that this field must be greater than the specified value, exclusive. If the value of Gt is larger than a specified Lt or Lte, the range is reversed. |
| gte | [uint64](#uint64) | optional | Gte specifies that this field must be greater than or equal to the specified value, inclusive. If the value of Gte is larger than a specified Lt or Lte, the range is reversed. |
| in | [uint64](#uint64) | repeated | In specifies that this field must be equal to one of the specified values |
| not_in | [uint64](#uint64) | repeated | NotIn specifies that this field cannot be equal to one of the specified values |
| ignore_empty | [bool](#bool) | optional | IgnoreEmpty specifies that the validation rules of this field should be evaluated only if the field is not empty |





 


<a name="validate-KnownRegex"></a>

### KnownRegex
WellKnownRegex contain some well-known patterns.

| Name | Number | Description |
| ---- | ------ | ----------- |
| UNKNOWN | 0 |  |
| HTTP_HEADER_NAME | 1 | HTTP header name as defined by RFC 7230. |
| HTTP_HEADER_VALUE | 2 | HTTP header value as defined by RFC 7230. |


 


<a name="gen_validate_validate-proto-extensions"></a>

### File-level Extensions
| Extension | Type | Base | Number | Description |
| --------- | ---- | ---- | ------ | ----------- |
| rules | FieldRules | .google.protobuf.FieldOptions | 1071 | Rules specify the validations to be performed on this field. By default, no validation is performed against a field. |
| disabled | bool | .google.protobuf.MessageOptions | 1071 | Disabled nullifies any validation rules for this message, including any message fields associated with it that do support validation. |
| ignored | bool | .google.protobuf.MessageOptions | 1072 | Ignore skips generation of validation methods for this message. |
| required | bool | .google.protobuf.OneofOptions | 1071 | Required ensures that exactly one the field options in a oneof is set; validation fails if no fields in the oneof are set. |

 

 



<a name="config-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## config.proto



<a name="stroppy-CloudConfig"></a>

### CloudConfig
CloudConfig contains configuration for stroppy cloud backend.






<a name="stroppy-ConfigFile"></a>

### ConfigFile
ConfigFile contains the complete configuration for a benchmark run in file.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| global | [GlobalConfig](#stroppy-GlobalConfig) |  | Global configuration |
| exporters | [ExporterConfig](#stroppy-ExporterConfig) | repeated | Exporters configuration |
| executors | [ExecutorConfig](#stroppy-ExecutorConfig) | repeated | Executors configuration |
| steps | [Step](#stroppy-Step) | repeated | Step to executor mapping configuration |
| side_cars | [SideCarConfig](#stroppy-SideCarConfig) | repeated | Plugins configuration |
| benchmark | [BenchmarkDescriptor](#stroppy-BenchmarkDescriptor) |  | BenchmarkDescriptor defines a complete benchmark consisting of multiple steps. |






<a name="stroppy-DriverConfig"></a>

### DriverConfig
DriverConfig contains configuration for connecting to a database driver.
It includes the driver plugin path, connection URL, and database-specific settings.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  | Database connection URL |
| db_specific | [Value.Struct](#stroppy-Value-Struct) | optional | Database-specific configuration options |
| driver_type | [DriverConfig.DriverType](#stroppy-DriverConfig-DriverType) |  | Name/Type of chosen driver |






<a name="stroppy-ExecutorConfig"></a>

### ExecutorConfig
ExecutorConfig contains configuration for an executor.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the executor |
| k6 | [K6Options](#stroppy-K6Options) |  | Configuration for the executor |






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
| driver | [DriverConfig](#stroppy-DriverConfig) |  | Database driver configuration |
| logger | [LoggerConfig](#stroppy-LoggerConfig) |  | Logging configuration |






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






<a name="stroppy-SideCarConfig"></a>

### SideCarConfig
SideCar contains configuration for plugins.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| url | [string](#string) |  | Url to connect the plugin instance |
| settings | [Value.Struct](#stroppy-Value-Struct) | optional | Additional plugin settings |






<a name="stroppy-Step"></a>

### Step
StepExecutorMappingConfig contains configuration for mapping steps to executors.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the step |
| workload | [string](#string) |  | Name of the step |
| executor | [string](#string) |  | Name of the executor |
| exporter | [string](#string) | optional | Name of the exporter |





 


<a name="stroppy-DriverConfig-DriverType"></a>

### DriverConfig.DriverType


| Name | Number | Description |
| ---- | ------ | ----------- |
| DRIVER_TYPE_UNSPECIFIED | 0 |  |
| DRIVER_TYPE_POSTGRES | 1 |  |



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


 

 

 



<a name="k6-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## k6.proto



<a name="stroppy-ConstantArrivalRate"></a>

### ConstantArrivalRate
ConstantArrivalRate executor configuration.
Documentation: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-arrival-rate/


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| rate | [uint32](#uint32) |  | Rate of iteration generation (number per time unit) |
| time_unit | [google.protobuf.Duration](#google-protobuf-Duration) |  | Time unit for the &#34;rate&#34; field (e.g., &#34;1s&#34;) |
| duration | [google.protobuf.Duration](#google-protobuf-Duration) |  | Duration of the scenario |
| pre_allocated_vus | [uint32](#uint32) |  | Number of VUs allocated in advance |
| max_vus | [uint32](#uint32) |  | Maximum allowed number of VUs if load increases |






<a name="stroppy-ConstantVUs"></a>

### ConstantVUs
ConstantVUs executor configuration.
Documentation: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/constant-vus/


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| vus | [uint32](#uint32) |  | Fixed number of virtual users that will be simultaneously active at all times |
| duration | [google.protobuf.Duration](#google-protobuf-Duration) |  | Duration of the scenario execution. All VUs will start and execute iterations until this time is completed. |






<a name="stroppy-K6Options"></a>

### K6Options
K6Executor contains configuration for k6 load testing tool integration.
It contains paths to the k6 binary and the k6 test script, as well as
additional arguments to pass to the k6 binary.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| k6_args | [string](#string) | repeated | Additional arguments to pass to the k6 binary |
| setup_timeout | [google.protobuf.Duration](#google-protobuf-Duration) | optional | Timeout for k6 setup phase |
| scenario | [K6Scenario](#stroppy-K6Scenario) |  | Scenario configuration |






<a name="stroppy-K6Scenario"></a>

### K6Scenario
Scenario defines the overall test scenario configuration.
It contains user tags, maximum duration, and executor configuration.
Documentation: https://grafana.com/docs/k6/latest/using-k6/scenarios/


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| max_duration | [google.protobuf.Duration](#google-protobuf-Duration) |  | Maximum duration for scenario execution. Used as a time limiter if main parameters (iterations, stages, duration) do not complete in time. |
| shared_iterations | [SharedIterations](#stroppy-SharedIterations) |  | Shared iterations executor |
| per_vu_iterations | [PerVuIterations](#stroppy-PerVuIterations) |  | Per-VU iterations executor |
| constant_vus | [ConstantVUs](#stroppy-ConstantVUs) |  | Constant VUs executor |
| ramping_vus | [RampingVUs](#stroppy-RampingVUs) |  | Ramping VUs executor |
| constant_arrival_rate | [ConstantArrivalRate](#stroppy-ConstantArrivalRate) |  | Constant arrival rate executor |
| ramping_arrival_rate | [RampingArrivalRate](#stroppy-RampingArrivalRate) |  | Ramping arrival rate executor |






<a name="stroppy-PerVuIterations"></a>

### PerVuIterations
PerVuIterations executor configuration.
Documentation: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/per-vu-iterations/


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| vus | [uint32](#uint32) |  | Number of virtual users |
| iterations | [int64](#int64) |  | Number of iterations that each VU should execute |






<a name="stroppy-RampingArrivalRate"></a>

### RampingArrivalRate
RampingArrivalRate executor configuration.
Documentation: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/ramping-arrival-rate/


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| start_rate | [uint32](#uint32) |  | Initial rate (iterations per time_unit) |
| time_unit | [google.protobuf.Duration](#google-protobuf-Duration) |  | Time unit for the rate (e.g., &#34;1s&#34;) |
| stages | [RampingArrivalRate.RateStage](#stroppy-RampingArrivalRate-RateStage) | repeated | List of rate change stages |
| pre_allocated_vus | [uint32](#uint32) |  | Number of VUs allocated in advance |
| max_vus | [uint32](#uint32) |  | Maximum number of VUs available for pool expansion |






<a name="stroppy-RampingArrivalRate-RateStage"></a>

### RampingArrivalRate.RateStage
Rate stage configuration for ramping arrival rate


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| target | [uint32](#uint32) |  | Target rate (iterations per time_unit) at the end of the stage |
| duration | [google.protobuf.Duration](#google-protobuf-Duration) |  | Duration of the stage |






<a name="stroppy-RampingVUs"></a>

### RampingVUs
RampingVUs executor configuration.
Documentation: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/ramping-vus/


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| start_vus | [uint32](#uint32) |  | Initial number of virtual users |
| stages | [RampingVUs.VUStage](#stroppy-RampingVUs-VUStage) | repeated | List of stages where VU count changes to target value over specified time |
| pre_allocated_vus | [uint32](#uint32) |  | Number of VUs allocated in advance. Helps avoid delays when creating new VUs during the test. |
| max_vus | [uint32](#uint32) |  | Maximum number of VUs available for pool expansion |






<a name="stroppy-RampingVUs-VUStage"></a>

### RampingVUs.VUStage
VU stage configuration for ramping


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| duration | [google.protobuf.Duration](#google-protobuf-Duration) |  | Duration of the stage (e.g., &#34;30s&#34;) |
| target | [uint32](#uint32) |  | Target number of VUs at the end of the stage |






<a name="stroppy-SharedIterations"></a>

### SharedIterations
SharedIterations executor configuration.
Documentation: https://grafana.com/docs/k6/latest/using-k6/scenarios/executors/shared-iterations/


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| iterations | [int64](#int64) |  | Total number of iterations to be executed by all VUs together. Iterations are distributed dynamically among available VUs. |
| vus | [uint32](#uint32) |  | Number of virtual users that will execute these iterations in parallel |





 

 

 

 



<a name="runtime-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## runtime.proto



<a name="stroppy-DriverQuery"></a>

### DriverQuery
DriverQuery represents a query that can be executed by a database driver.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the query |
| request | [string](#string) |  | Request of the query |
| params | [Value](#stroppy-Value) | repeated | Parameters of the query |






<a name="stroppy-DriverQueryStat"></a>

### DriverQueryStat



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



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| queries | [DriverQueryStat](#stroppy-DriverQueryStat) | repeated |  |
| exec_duration | [google.protobuf.Duration](#google-protobuf-Duration) |  |  |
| isolation_level | [TxIsolationLevel](#stroppy-TxIsolationLevel) |  |  |






<a name="stroppy-StepContext"></a>

### StepContext
StepContext provides contextual information to a benchmark step during
execution. It contains the run context and the step descriptor.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| config | [GlobalConfig](#stroppy-GlobalConfig) |  | Global configuration of the benchmark and its steps |
| step | [Step](#stroppy-Step) |  | Current step |
| executor | [ExecutorConfig](#stroppy-ExecutorConfig) |  | Executor configuration |
| exporter | [ExporterConfig](#stroppy-ExporterConfig) | optional | Exporter configuration |
| workload | [WorkloadDescriptor](#stroppy-WorkloadDescriptor) |  | Current workload descriptor |






<a name="stroppy-UnitContext"></a>

### UnitContext
UnitBuildContext provides the context needed to build a unit from a
WorkloadUnitDescriptor.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| step_context | [StepContext](#stroppy-StepContext) |  | Step context |
| unit_descriptor | [WorkloadUnitDescriptor](#stroppy-WorkloadUnitDescriptor) |  | Current unit descriptor |





 

 

 

 



<a name="sidecar-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## sidecar.proto


 

 

 


<a name="stroppy-SidecarService"></a>

### SidecarService
SidecarPlugin defines the gRPC service that sidecar plugins must implement.

| Method Name | Request Type | Response Type | Description |
| ----------- | ------------ | ------------- | ------------|
| Initialize | [GlobalConfig](#stroppy-GlobalConfig) | [.google.protobuf.Empty](#google-protobuf-Empty) | Initialize is called once before the benchmark starts. Used to initialize resources of SidecarPlugin. |
| OnStepStart | [StepContext](#stroppy-StepContext) | [.google.protobuf.Empty](#google-protobuf-Empty) | OnStepStart is called once before each step starts. |
| OnStepEnd | [StepContext](#stroppy-StepContext) | [.google.protobuf.Empty](#google-protobuf-Empty) | OnStepEnd is called once after each step ends. |
| Teardown | [GlobalConfig](#stroppy-GlobalConfig) | [.google.protobuf.Empty](#google-protobuf-Empty) | Teardown is called once after the benchmark ends. Used to clean up resources. |

 



<a name="common-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## common.proto



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
| ranges | [Generation.Range.UInt32Range](#stroppy-Generation-Range-UInt32Range) | repeated | List of character ranges for this alphabet |






<a name="stroppy-Generation-Distribution"></a>

### Generation.Distribution
Distribution defines the statistical distribution for value generation.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| type | [Generation.Distribution.DistributionType](#stroppy-Generation-Distribution-DistributionType) |  | Type of distribution to use |
| screw | [double](#double) |  | Distribution parameter (e.g., standard deviation for normal distribution) |






<a name="stroppy-Generation-Range"></a>

### Generation.Range
Range defines value constraints for generation.






<a name="stroppy-Generation-Range-AnyStringRange"></a>

### Generation.Range.AnyStringRange
Range for string values that can be parsed into other types


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [string](#string) |  | Minimum value (inclusive) |
| max | [string](#string) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-DateTimeRange"></a>

### Generation.Range.DateTimeRange
Range for date/time values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| default | [Generation.Range.DateTimeRange.Default](#stroppy-Generation-Range-DateTimeRange-Default) |  | Exact date/time range |
| string | [Generation.Range.AnyStringRange](#stroppy-Generation-Range-AnyStringRange) |  | String-based range (ISO 8601 format) |
| timestamp_pb | [Generation.Range.DateTimeRange.TimestampPb](#stroppy-Generation-Range-DateTimeRange-TimestampPb) |  | Protocol Buffers timestamp range |
| timestamp | [Generation.Range.DateTimeRange.Timestamp](#stroppy-Generation-Range-DateTimeRange-Timestamp) |  | Unix timestamp range |






<a name="stroppy-Generation-Range-DateTimeRange-Default"></a>

### Generation.Range.DateTimeRange.Default
Default date/time range


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [DateTime](#stroppy-DateTime) |  | Minimum timestamp (inclusive) |
| max | [DateTime](#stroppy-DateTime) |  | Maximum timestamp (inclusive) |






<a name="stroppy-Generation-Range-DateTimeRange-Timestamp"></a>

### Generation.Range.DateTimeRange.Timestamp
Unix timestamp range


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [uint32](#uint32) |  | Minimum Unix timestamp (inclusive) |
| max | [uint32](#uint32) |  | Maximum Unix timestamp (inclusive) |






<a name="stroppy-Generation-Range-DateTimeRange-TimestampPb"></a>

### Generation.Range.DateTimeRange.TimestampPb
Protocol Buffers timestamp range


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | Minimum timestamp (inclusive) |
| max | [google.protobuf.Timestamp](#google-protobuf-Timestamp) |  | Maximum timestamp (inclusive) |






<a name="stroppy-Generation-Range-DecimalRange"></a>

### Generation.Range.DecimalRange
Range for decimal numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| default | [Generation.Range.DecimalRange.Default](#stroppy-Generation-Range-DecimalRange-Default) |  | Exact decimal range |
| float | [Generation.Range.FloatRange](#stroppy-Generation-Range-FloatRange) |  | Float-based range |
| double | [Generation.Range.DoubleRange](#stroppy-Generation-Range-DoubleRange) |  | Double-based range |
| string | [Generation.Range.AnyStringRange](#stroppy-Generation-Range-AnyStringRange) |  | String-based range (supports scientific notation) |






<a name="stroppy-Generation-Range-DecimalRange-Default"></a>

### Generation.Range.DecimalRange.Default
Default decimal range


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [Decimal](#stroppy-Decimal) |  | Minimum value (inclusive) |
| max | [Decimal](#stroppy-Decimal) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-DoubleRange"></a>

### Generation.Range.DoubleRange
Range for 64-bit floating point numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [double](#double) |  | Minimum value (inclusive) |
| max | [double](#double) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-FloatRange"></a>

### Generation.Range.FloatRange
Range for 32-bit floating point numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [float](#float) |  | Minimum value (inclusive) |
| max | [float](#float) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-Int32Range"></a>

### Generation.Range.Int32Range
Range for 32-bit signed integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [int32](#int32) |  | Minimum value (inclusive) |
| max | [int32](#int32) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-Int64Range"></a>

### Generation.Range.Int64Range
Range for 64-bit signed integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [int64](#int64) |  | Minimum value (inclusive) |
| max | [int64](#int64) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-UInt32Range"></a>

### Generation.Range.UInt32Range
Range for 32-bit unsigned integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [uint32](#uint32) |  | Minimum value (inclusive) |
| max | [uint32](#uint32) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Range-UInt64Range"></a>

### Generation.Range.UInt64Range
Range for 64-bit unsigned integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| min | [uint64](#uint64) |  | Minimum value (inclusive) |
| max | [uint64](#uint64) |  | Maximum value (inclusive) |






<a name="stroppy-Generation-Rule"></a>

### Generation.Rule
Rule defines generation rules for a specific data type.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| float_rules | [Generation.Rules.FloatRule](#stroppy-Generation-Rules-FloatRule) |  | Rules for 32-bit floating point numbers |
| double_rules | [Generation.Rules.DoubleRule](#stroppy-Generation-Rules-DoubleRule) |  | Rules for 64-bit floating point numbers |
| int32_rules | [Generation.Rules.Int32Rule](#stroppy-Generation-Rules-Int32Rule) |  | Rules for 32-bit signed integers |
| int64_rules | [Generation.Rules.Int64Rule](#stroppy-Generation-Rules-Int64Rule) |  | Rules for 64-bit signed integers |
| uint32_rules | [Generation.Rules.UInt32Rule](#stroppy-Generation-Rules-UInt32Rule) |  | Rules for 32-bit unsigned integers |
| uint64_rules | [Generation.Rules.UInt64Rule](#stroppy-Generation-Rules-UInt64Rule) |  | Rules for 64-bit unsigned integers |
| bool_rules | [Generation.Rules.BoolRule](#stroppy-Generation-Rules-BoolRule) |  | Rules for boolean values |
| string_rules | [Generation.Rules.StringRule](#stroppy-Generation-Rules-StringRule) |  | Rules for string values |
| datetime_rules | [Generation.Rules.DateTimeRule](#stroppy-Generation-Rules-DateTimeRule) |  | Rules for date/time values |
| uuid_rules | [Generation.Rules.UuidRule](#stroppy-Generation-Rules-UuidRule) |  | Rules for UUIDs |
| decimal_rules | [Generation.Rules.DecimalRule](#stroppy-Generation-Rules-DecimalRule) |  | Rules for decimal numbers |
| distribution | [Generation.Distribution](#stroppy-Generation-Distribution) | optional |  |
| null_percentage | [uint32](#uint32) | optional |  |
| unique | [bool](#bool) | optional |  |






<a name="stroppy-Generation-Rules"></a>

### Generation.Rules
Rules contains type-specific generation configurations.






<a name="stroppy-Generation-Rules-BoolRule"></a>

### Generation.Rules.BoolRule
Rules for generating boolean values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| constant | [bool](#bool) | optional | Fixed value (if not specified, generates random booleans) |






<a name="stroppy-Generation-Rules-DateTimeRule"></a>

### Generation.Rules.DateTimeRule
Rules for generating date/time values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Generation.Range.DateTimeRange](#stroppy-Generation-Range-DateTimeRange) |  | Valid time range |
| constant | [DateTime](#stroppy-DateTime) | optional | Fixed value (if specified, overrides range) |






<a name="stroppy-Generation-Rules-DecimalRule"></a>

### Generation.Rules.DecimalRule
Rules for generating decimal numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Generation.Range.DecimalRange](#stroppy-Generation-Range-DecimalRange) |  | Valid value range |
| constant | [Decimal](#stroppy-Decimal) | optional | Fixed value (if specified, overrides range) |






<a name="stroppy-Generation-Rules-DoubleRule"></a>

### Generation.Rules.DoubleRule
Rules for generating 64-bit floating point numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Generation.Range.DoubleRange](#stroppy-Generation-Range-DoubleRange) |  | Valid value range |
| constant | [double](#double) | optional | Fixed value (if specified, overrides range) |






<a name="stroppy-Generation-Rules-FloatRule"></a>

### Generation.Rules.FloatRule
Rules for generating 32-bit floating point numbers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Generation.Range.FloatRange](#stroppy-Generation-Range-FloatRange) |  | Valid value range |
| constant | [float](#float) | optional | Fixed value (if specified, overrides range) |






<a name="stroppy-Generation-Rules-Int32Rule"></a>

### Generation.Rules.Int32Rule
Rules for generating 32-bit signed integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Generation.Range.Int32Range](#stroppy-Generation-Range-Int32Range) |  | Valid value range |
| constant | [int32](#int32) | optional | Fixed value (if specified, overrides range) |






<a name="stroppy-Generation-Rules-Int64Rule"></a>

### Generation.Rules.Int64Rule
Rules for generating 64-bit signed integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Generation.Range.Int64Range](#stroppy-Generation-Range-Int64Range) |  | Valid value range |
| constant | [int64](#int64) | optional | Fixed value (if specified, overrides range) |






<a name="stroppy-Generation-Rules-StringRule"></a>

### Generation.Rules.StringRule
Rules for generating string values


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| alphabet | [Generation.Alphabet](#stroppy-Generation-Alphabet) | optional | Character set to use for generation |
| len_range | [Generation.Range.UInt64Range](#stroppy-Generation-Range-UInt64Range) |  | Valid length range for the string |
| constant | [string](#string) | optional | Fixed value (if specified, overrides generation) |






<a name="stroppy-Generation-Rules-UInt32Rule"></a>

### Generation.Rules.UInt32Rule
Rules for generating 32-bit unsigned integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Generation.Range.UInt32Range](#stroppy-Generation-Range-UInt32Range) |  | Valid value range |
| constant | [uint32](#uint32) | optional | Fixed value (if specified, overrides range) |






<a name="stroppy-Generation-Rules-UInt64Rule"></a>

### Generation.Rules.UInt64Rule
Rules for generating 64-bit unsigned integers


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| range | [Generation.Range.UInt64Range](#stroppy-Generation-Range-UInt64Range) |  | Valid value range |
| constant | [uint64](#uint64) | optional | Fixed value (if specified, overrides range) |






<a name="stroppy-Generation-Rules-UuidRule"></a>

### Generation.Rules.UuidRule
Rules for generating UUIDs


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| constant | [Uuid](#stroppy-Uuid) | optional | Fixed UUID (if not specified, generates random UUIDs) |






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



<a name="stroppy-Value-NullValue"></a>

### Value.NullValue


| Name | Number | Description |
| ---- | ------ | ----------- |
| NULL_VALUE | 0 | Null value |


 

 

 



<a name="descriptor-proto"></a>
<p align="right"><a href="#top">Top</a></p>

## descriptor.proto



<a name="stroppy-BenchmarkDescriptor"></a>

### BenchmarkDescriptor
BenchmarkDescriptor defines a complete benchmark consisting of multiple
steps.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the benchmark |
| workloads | [WorkloadDescriptor](#stroppy-WorkloadDescriptor) | repeated | List of steps to execute in the benchmark |






<a name="stroppy-ColumnDescriptor"></a>

### ColumnDescriptor
ColumnDescriptor defines the structure of a database column.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the column |
| sql_type | [string](#string) |  | SQL data type of the column |
| nullable | [bool](#bool) |  | Whether the column can be NULL |
| primary_key | [bool](#bool) |  | Whether the column is part of the primary key |
| unique | [bool](#bool) |  | Whether the column has a UNIQUE constraint |
| constraint | [string](#string) |  | SQL constraint definition for the column |






<a name="stroppy-IndexDescriptor"></a>

### IndexDescriptor
IndexDescriptor defines the structure of a database index.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the index |
| columns | [string](#string) | repeated | List of column names that are part of this index |
| type | [string](#string) |  | Type of index (e.g., BTREE, HASH, etc.) |
| unique | [bool](#bool) |  | Whether this is a unique index |
| db_specific | [Value.Struct](#stroppy-Value-Struct) | optional | Database-specific index properties |






<a name="stroppy-QueryDescriptor"></a>

### QueryDescriptor
QueryDescriptor defines a database query with its parameters and execution
count.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the query |
| sql | [string](#string) |  | SQL query text |
| params | [QueryParamDescriptor](#stroppy-QueryParamDescriptor) | repeated | Parameters used in the query |
| groups | [QueryParamGroup](#stroppy-QueryParamGroup) | repeated |  |
| db_specific | [Value.Struct](#stroppy-Value-Struct) | optional | Database-specific query properties |






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



| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  |  |
| params | [QueryParamDescriptor](#stroppy-QueryParamDescriptor) | repeated |  |






<a name="stroppy-TableDescriptor"></a>

### TableDescriptor
TableDescriptor defines the structure of a database table.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the table |
| table_indexes | [IndexDescriptor](#stroppy-IndexDescriptor) | repeated | List of indexes defined on this table |
| constraint | [string](#string) |  | Table-level constraints |
| db_specific | [Value.Struct](#stroppy-Value-Struct) |  | Database-specific table properties |
| columns | [ColumnDescriptor](#stroppy-ColumnDescriptor) | repeated | Columns defined in this table |






<a name="stroppy-TransactionDescriptor"></a>

### TransactionDescriptor
TransactionDescriptor defines a database transaction with its queries and
execution count.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the transaction |
| isolation_level | [TxIsolationLevel](#stroppy-TxIsolationLevel) |  | Transaction isolation level |
| queries | [QueryDescriptor](#stroppy-QueryDescriptor) | repeated | List of queries to execute in this transaction |
| db_specific | [Value.Struct](#stroppy-Value-Struct) | optional | Database-specific transaction properties |






<a name="stroppy-UnitDescriptor"></a>

### UnitDescriptor
UnitDescriptor represents a single workload.
It can be a table creation operation, a query execution operation, or a
transaction execution operation.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| create_table | [TableDescriptor](#stroppy-TableDescriptor) |  | Table creation operation |
| query | [QueryDescriptor](#stroppy-QueryDescriptor) |  | Query execution operation |
| transaction | [TransactionDescriptor](#stroppy-TransactionDescriptor) |  | Transaction execution operation |






<a name="stroppy-WorkloadDescriptor"></a>

### WorkloadDescriptor
WorkloadDescriptor represents a logical step in a benchmark.
It contains a list of operations to perform in this step.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| name | [string](#string) |  | Name of the step |
| async | [bool](#bool) |  | Whether to execute all operations in this workload asynchronously |
| units | [WorkloadUnitDescriptor](#stroppy-WorkloadUnitDescriptor) | repeated | List of operations to perform in this step |






<a name="stroppy-WorkloadUnitDescriptor"></a>

### WorkloadUnitDescriptor
WorkloadUnitDescriptor represents a single unit of work.
It can be a table creation operation, a query execution operation, or a
transaction execution operation.


| Field | Type | Label | Description |
| ----- | ---- | ----- | ----------- |
| descriptor | [UnitDescriptor](#stroppy-UnitDescriptor) |  |  |
| count | [uint64](#uint64) |  | Number of times to execute this unit |





 


<a name="stroppy-TxIsolationLevel"></a>

### TxIsolationLevel
TransactionIsolationLevel defines the isolation level for a database
transaction.

| Name | Number | Description |
| ---- | ------ | ----------- |
| TX_ISOLATION_LEVEL_UNSPECIFIED | 0 |  |
| TX_ISOLATION_LEVEL_READ_UNCOMMITTED | 1 |  |
| TX_ISOLATION_LEVEL_READ_COMMITTED | 2 |  |
| TX_ISOLATION_LEVEL_REPEATABLE_READ | 3 |  |
| TX_ISOLATION_LEVEL_SERIALIZABLE | 4 |  |


 

 

 



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

