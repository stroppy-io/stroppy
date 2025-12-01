// node_modules/@protobuf-ts/runtime/build/es2015/json-typings.js
function typeofJsonValue(value) {
  let t = typeof value;
  if (t == "object") {
    if (Array.isArray(value))
      return "array";
    if (value === null)
      return "null";
  }
  return t;
}
function isJsonObject(value) {
  return value !== null && typeof value == "object" && !Array.isArray(value);
}

// node_modules/@protobuf-ts/runtime/build/es2015/base64.js
var encTable = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".split("");
var decTable = [];
for (let i = 0; i < encTable.length; i++)
  decTable[encTable[i].charCodeAt(0)] = i;
decTable["-".charCodeAt(0)] = encTable.indexOf("+");
decTable["_".charCodeAt(0)] = encTable.indexOf("/");
function base64decode(base64Str) {
  let es = base64Str.length * 3 / 4;
  if (base64Str[base64Str.length - 2] == "=")
    es -= 2;
  else if (base64Str[base64Str.length - 1] == "=")
    es -= 1;
  let bytes = new Uint8Array(es), bytePos = 0, groupPos = 0, b, p = 0;
  for (let i = 0; i < base64Str.length; i++) {
    b = decTable[base64Str.charCodeAt(i)];
    if (b === void 0) {
      switch (base64Str[i]) {
        case "=":
          groupPos = 0;
        // reset state when padding found
        case "\n":
        case "\r":
        case "	":
        case " ":
          continue;
        // skip white-space, and padding
        default:
          throw Error(`invalid base64 string.`);
      }
    }
    switch (groupPos) {
      case 0:
        p = b;
        groupPos = 1;
        break;
      case 1:
        bytes[bytePos++] = p << 2 | (b & 48) >> 4;
        p = b;
        groupPos = 2;
        break;
      case 2:
        bytes[bytePos++] = (p & 15) << 4 | (b & 60) >> 2;
        p = b;
        groupPos = 3;
        break;
      case 3:
        bytes[bytePos++] = (p & 3) << 6 | b;
        groupPos = 0;
        break;
    }
  }
  if (groupPos == 1)
    throw Error(`invalid base64 string.`);
  return bytes.subarray(0, bytePos);
}
function base64encode(bytes) {
  let base64 = "", groupPos = 0, b, p = 0;
  for (let i = 0; i < bytes.length; i++) {
    b = bytes[i];
    switch (groupPos) {
      case 0:
        base64 += encTable[b >> 2];
        p = (b & 3) << 4;
        groupPos = 1;
        break;
      case 1:
        base64 += encTable[p | b >> 4];
        p = (b & 15) << 2;
        groupPos = 2;
        break;
      case 2:
        base64 += encTable[p | b >> 6];
        base64 += encTable[b & 63];
        groupPos = 0;
        break;
    }
  }
  if (groupPos) {
    base64 += encTable[p];
    base64 += "=";
    if (groupPos == 1)
      base64 += "=";
  }
  return base64;
}

// node_modules/@protobuf-ts/runtime/build/es2015/binary-format-contract.js
var UnknownFieldHandler;
(function(UnknownFieldHandler2) {
  UnknownFieldHandler2.symbol = Symbol.for("protobuf-ts/unknown");
  UnknownFieldHandler2.onRead = (typeName, message, fieldNo, wireType, data) => {
    let container = is(message) ? message[UnknownFieldHandler2.symbol] : message[UnknownFieldHandler2.symbol] = [];
    container.push({ no: fieldNo, wireType, data });
  };
  UnknownFieldHandler2.onWrite = (typeName, message, writer) => {
    for (let { no, wireType, data } of UnknownFieldHandler2.list(message))
      writer.tag(no, wireType).raw(data);
  };
  UnknownFieldHandler2.list = (message, fieldNo) => {
    if (is(message)) {
      let all = message[UnknownFieldHandler2.symbol];
      return fieldNo ? all.filter((uf) => uf.no == fieldNo) : all;
    }
    return [];
  };
  UnknownFieldHandler2.last = (message, fieldNo) => UnknownFieldHandler2.list(message, fieldNo).slice(-1)[0];
  const is = (message) => message && Array.isArray(message[UnknownFieldHandler2.symbol]);
})(UnknownFieldHandler || (UnknownFieldHandler = {}));
var WireType;
(function(WireType2) {
  WireType2[WireType2["Varint"] = 0] = "Varint";
  WireType2[WireType2["Bit64"] = 1] = "Bit64";
  WireType2[WireType2["LengthDelimited"] = 2] = "LengthDelimited";
  WireType2[WireType2["StartGroup"] = 3] = "StartGroup";
  WireType2[WireType2["EndGroup"] = 4] = "EndGroup";
  WireType2[WireType2["Bit32"] = 5] = "Bit32";
})(WireType || (WireType = {}));

// node_modules/@protobuf-ts/runtime/build/es2015/goog-varint.js
function varint64read() {
  let lowBits = 0;
  let highBits = 0;
  for (let shift = 0; shift < 28; shift += 7) {
    let b = this.buf[this.pos++];
    lowBits |= (b & 127) << shift;
    if ((b & 128) == 0) {
      this.assertBounds();
      return [lowBits, highBits];
    }
  }
  let middleByte = this.buf[this.pos++];
  lowBits |= (middleByte & 15) << 28;
  highBits = (middleByte & 112) >> 4;
  if ((middleByte & 128) == 0) {
    this.assertBounds();
    return [lowBits, highBits];
  }
  for (let shift = 3; shift <= 31; shift += 7) {
    let b = this.buf[this.pos++];
    highBits |= (b & 127) << shift;
    if ((b & 128) == 0) {
      this.assertBounds();
      return [lowBits, highBits];
    }
  }
  throw new Error("invalid varint");
}
function varint64write(lo, hi, bytes) {
  for (let i = 0; i < 28; i = i + 7) {
    const shift = lo >>> i;
    const hasNext = !(shift >>> 7 == 0 && hi == 0);
    const byte = (hasNext ? shift | 128 : shift) & 255;
    bytes.push(byte);
    if (!hasNext) {
      return;
    }
  }
  const splitBits = lo >>> 28 & 15 | (hi & 7) << 4;
  const hasMoreBits = !(hi >> 3 == 0);
  bytes.push((hasMoreBits ? splitBits | 128 : splitBits) & 255);
  if (!hasMoreBits) {
    return;
  }
  for (let i = 3; i < 31; i = i + 7) {
    const shift = hi >>> i;
    const hasNext = !(shift >>> 7 == 0);
    const byte = (hasNext ? shift | 128 : shift) & 255;
    bytes.push(byte);
    if (!hasNext) {
      return;
    }
  }
  bytes.push(hi >>> 31 & 1);
}
var TWO_PWR_32_DBL = (1 << 16) * (1 << 16);
function int64fromString(dec) {
  let minus = dec[0] == "-";
  if (minus)
    dec = dec.slice(1);
  const base = 1e6;
  let lowBits = 0;
  let highBits = 0;
  function add1e6digit(begin, end) {
    const digit1e6 = Number(dec.slice(begin, end));
    highBits *= base;
    lowBits = lowBits * base + digit1e6;
    if (lowBits >= TWO_PWR_32_DBL) {
      highBits = highBits + (lowBits / TWO_PWR_32_DBL | 0);
      lowBits = lowBits % TWO_PWR_32_DBL;
    }
  }
  add1e6digit(-24, -18);
  add1e6digit(-18, -12);
  add1e6digit(-12, -6);
  add1e6digit(-6);
  return [minus, lowBits, highBits];
}
function int64toString(bitsLow, bitsHigh) {
  if (bitsHigh >>> 0 <= 2097151) {
    return "" + (TWO_PWR_32_DBL * bitsHigh + (bitsLow >>> 0));
  }
  let low = bitsLow & 16777215;
  let mid = (bitsLow >>> 24 | bitsHigh << 8) >>> 0 & 16777215;
  let high = bitsHigh >> 16 & 65535;
  let digitA = low + mid * 6777216 + high * 6710656;
  let digitB = mid + high * 8147497;
  let digitC = high * 2;
  let base = 1e7;
  if (digitA >= base) {
    digitB += Math.floor(digitA / base);
    digitA %= base;
  }
  if (digitB >= base) {
    digitC += Math.floor(digitB / base);
    digitB %= base;
  }
  function decimalFrom1e7(digit1e7, needLeadingZeros) {
    let partial = digit1e7 ? String(digit1e7) : "";
    if (needLeadingZeros) {
      return "0000000".slice(partial.length) + partial;
    }
    return partial;
  }
  return decimalFrom1e7(
    digitC,
    /*needLeadingZeros=*/
    0
  ) + decimalFrom1e7(
    digitB,
    /*needLeadingZeros=*/
    digitC
  ) + // If the final 1e7 digit didn't need leading zeros, we would have
  // returned via the trivial code path at the top.
  decimalFrom1e7(
    digitA,
    /*needLeadingZeros=*/
    1
  );
}
function varint32write(value, bytes) {
  if (value >= 0) {
    while (value > 127) {
      bytes.push(value & 127 | 128);
      value = value >>> 7;
    }
    bytes.push(value);
  } else {
    for (let i = 0; i < 9; i++) {
      bytes.push(value & 127 | 128);
      value = value >> 7;
    }
    bytes.push(1);
  }
}
function varint32read() {
  let b = this.buf[this.pos++];
  let result = b & 127;
  if ((b & 128) == 0) {
    this.assertBounds();
    return result;
  }
  b = this.buf[this.pos++];
  result |= (b & 127) << 7;
  if ((b & 128) == 0) {
    this.assertBounds();
    return result;
  }
  b = this.buf[this.pos++];
  result |= (b & 127) << 14;
  if ((b & 128) == 0) {
    this.assertBounds();
    return result;
  }
  b = this.buf[this.pos++];
  result |= (b & 127) << 21;
  if ((b & 128) == 0) {
    this.assertBounds();
    return result;
  }
  b = this.buf[this.pos++];
  result |= (b & 15) << 28;
  for (let readBytes = 5; (b & 128) !== 0 && readBytes < 10; readBytes++)
    b = this.buf[this.pos++];
  if ((b & 128) != 0)
    throw new Error("invalid varint");
  this.assertBounds();
  return result >>> 0;
}

// node_modules/@protobuf-ts/runtime/build/es2015/pb-long.js
var BI;
function detectBi() {
  const dv = new DataView(new ArrayBuffer(8));
  const ok = globalThis.BigInt !== void 0 && typeof dv.getBigInt64 === "function" && typeof dv.getBigUint64 === "function" && typeof dv.setBigInt64 === "function" && typeof dv.setBigUint64 === "function";
  BI = ok ? {
    MIN: BigInt("-9223372036854775808"),
    MAX: BigInt("9223372036854775807"),
    UMIN: BigInt("0"),
    UMAX: BigInt("18446744073709551615"),
    C: BigInt,
    V: dv
  } : void 0;
}
detectBi();
function assertBi(bi) {
  if (!bi)
    throw new Error("BigInt unavailable, see https://github.com/timostamm/protobuf-ts/blob/v1.0.8/MANUAL.md#bigint-support");
}
var RE_DECIMAL_STR = /^-?[0-9]+$/;
var TWO_PWR_32_DBL2 = 4294967296;
var HALF_2_PWR_32 = 2147483648;
var SharedPbLong = class {
  /**
   * Create a new instance with the given bits.
   */
  constructor(lo, hi) {
    this.lo = lo | 0;
    this.hi = hi | 0;
  }
  /**
   * Is this instance equal to 0?
   */
  isZero() {
    return this.lo == 0 && this.hi == 0;
  }
  /**
   * Convert to a native number.
   */
  toNumber() {
    let result = this.hi * TWO_PWR_32_DBL2 + (this.lo >>> 0);
    if (!Number.isSafeInteger(result))
      throw new Error("cannot convert to safe number");
    return result;
  }
};
var PbULong = class _PbULong extends SharedPbLong {
  /**
   * Create instance from a `string`, `number` or `bigint`.
   */
  static from(value) {
    if (BI)
      switch (typeof value) {
        case "string":
          if (value == "0")
            return this.ZERO;
          if (value == "")
            throw new Error("string is no integer");
          value = BI.C(value);
        case "number":
          if (value === 0)
            return this.ZERO;
          value = BI.C(value);
        case "bigint":
          if (!value)
            return this.ZERO;
          if (value < BI.UMIN)
            throw new Error("signed value for ulong");
          if (value > BI.UMAX)
            throw new Error("ulong too large");
          BI.V.setBigUint64(0, value, true);
          return new _PbULong(BI.V.getInt32(0, true), BI.V.getInt32(4, true));
      }
    else
      switch (typeof value) {
        case "string":
          if (value == "0")
            return this.ZERO;
          value = value.trim();
          if (!RE_DECIMAL_STR.test(value))
            throw new Error("string is no integer");
          let [minus, lo, hi] = int64fromString(value);
          if (minus)
            throw new Error("signed value for ulong");
          return new _PbULong(lo, hi);
        case "number":
          if (value == 0)
            return this.ZERO;
          if (!Number.isSafeInteger(value))
            throw new Error("number is no integer");
          if (value < 0)
            throw new Error("signed value for ulong");
          return new _PbULong(value, value / TWO_PWR_32_DBL2);
      }
    throw new Error("unknown value " + typeof value);
  }
  /**
   * Convert to decimal string.
   */
  toString() {
    return BI ? this.toBigInt().toString() : int64toString(this.lo, this.hi);
  }
  /**
   * Convert to native bigint.
   */
  toBigInt() {
    assertBi(BI);
    BI.V.setInt32(0, this.lo, true);
    BI.V.setInt32(4, this.hi, true);
    return BI.V.getBigUint64(0, true);
  }
};
PbULong.ZERO = new PbULong(0, 0);
var PbLong = class _PbLong extends SharedPbLong {
  /**
   * Create instance from a `string`, `number` or `bigint`.
   */
  static from(value) {
    if (BI)
      switch (typeof value) {
        case "string":
          if (value == "0")
            return this.ZERO;
          if (value == "")
            throw new Error("string is no integer");
          value = BI.C(value);
        case "number":
          if (value === 0)
            return this.ZERO;
          value = BI.C(value);
        case "bigint":
          if (!value)
            return this.ZERO;
          if (value < BI.MIN)
            throw new Error("signed long too small");
          if (value > BI.MAX)
            throw new Error("signed long too large");
          BI.V.setBigInt64(0, value, true);
          return new _PbLong(BI.V.getInt32(0, true), BI.V.getInt32(4, true));
      }
    else
      switch (typeof value) {
        case "string":
          if (value == "0")
            return this.ZERO;
          value = value.trim();
          if (!RE_DECIMAL_STR.test(value))
            throw new Error("string is no integer");
          let [minus, lo, hi] = int64fromString(value);
          if (minus) {
            if (hi > HALF_2_PWR_32 || hi == HALF_2_PWR_32 && lo != 0)
              throw new Error("signed long too small");
          } else if (hi >= HALF_2_PWR_32)
            throw new Error("signed long too large");
          let pbl = new _PbLong(lo, hi);
          return minus ? pbl.negate() : pbl;
        case "number":
          if (value == 0)
            return this.ZERO;
          if (!Number.isSafeInteger(value))
            throw new Error("number is no integer");
          return value > 0 ? new _PbLong(value, value / TWO_PWR_32_DBL2) : new _PbLong(-value, -value / TWO_PWR_32_DBL2).negate();
      }
    throw new Error("unknown value " + typeof value);
  }
  /**
   * Do we have a minus sign?
   */
  isNegative() {
    return (this.hi & HALF_2_PWR_32) !== 0;
  }
  /**
   * Negate two's complement.
   * Invert all the bits and add one to the result.
   */
  negate() {
    let hi = ~this.hi, lo = this.lo;
    if (lo)
      lo = ~lo + 1;
    else
      hi += 1;
    return new _PbLong(lo, hi);
  }
  /**
   * Convert to decimal string.
   */
  toString() {
    if (BI)
      return this.toBigInt().toString();
    if (this.isNegative()) {
      let n = this.negate();
      return "-" + int64toString(n.lo, n.hi);
    }
    return int64toString(this.lo, this.hi);
  }
  /**
   * Convert to native bigint.
   */
  toBigInt() {
    assertBi(BI);
    BI.V.setInt32(0, this.lo, true);
    BI.V.setInt32(4, this.hi, true);
    return BI.V.getBigInt64(0, true);
  }
};
PbLong.ZERO = new PbLong(0, 0);

// node_modules/@protobuf-ts/runtime/build/es2015/binary-reader.js
var defaultsRead = {
  readUnknownField: true,
  readerFactory: (bytes) => new BinaryReader(bytes)
};
function binaryReadOptions(options) {
  return options ? Object.assign(Object.assign({}, defaultsRead), options) : defaultsRead;
}
var BinaryReader = class {
  constructor(buf, textDecoder) {
    this.varint64 = varint64read;
    this.uint32 = varint32read;
    this.buf = buf;
    this.len = buf.length;
    this.pos = 0;
    this.view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
    this.textDecoder = textDecoder !== null && textDecoder !== void 0 ? textDecoder : new TextDecoder("utf-8", {
      fatal: true,
      ignoreBOM: true
    });
  }
  /**
   * Reads a tag - field number and wire type.
   */
  tag() {
    let tag = this.uint32(), fieldNo = tag >>> 3, wireType = tag & 7;
    if (fieldNo <= 0 || wireType < 0 || wireType > 5)
      throw new Error("illegal tag: field no " + fieldNo + " wire type " + wireType);
    return [fieldNo, wireType];
  }
  /**
   * Skip one element on the wire and return the skipped data.
   * Supports WireType.StartGroup since v2.0.0-alpha.23.
   */
  skip(wireType) {
    let start = this.pos;
    switch (wireType) {
      case WireType.Varint:
        while (this.buf[this.pos++] & 128) {
        }
        break;
      case WireType.Bit64:
        this.pos += 4;
      case WireType.Bit32:
        this.pos += 4;
        break;
      case WireType.LengthDelimited:
        let len = this.uint32();
        this.pos += len;
        break;
      case WireType.StartGroup:
        let t;
        while ((t = this.tag()[1]) !== WireType.EndGroup) {
          this.skip(t);
        }
        break;
      default:
        throw new Error("cant skip wire type " + wireType);
    }
    this.assertBounds();
    return this.buf.subarray(start, this.pos);
  }
  /**
   * Throws error if position in byte array is out of range.
   */
  assertBounds() {
    if (this.pos > this.len)
      throw new RangeError("premature EOF");
  }
  /**
   * Read a `int32` field, a signed 32 bit varint.
   */
  int32() {
    return this.uint32() | 0;
  }
  /**
   * Read a `sint32` field, a signed, zigzag-encoded 32-bit varint.
   */
  sint32() {
    let zze = this.uint32();
    return zze >>> 1 ^ -(zze & 1);
  }
  /**
   * Read a `int64` field, a signed 64-bit varint.
   */
  int64() {
    return new PbLong(...this.varint64());
  }
  /**
   * Read a `uint64` field, an unsigned 64-bit varint.
   */
  uint64() {
    return new PbULong(...this.varint64());
  }
  /**
   * Read a `sint64` field, a signed, zig-zag-encoded 64-bit varint.
   */
  sint64() {
    let [lo, hi] = this.varint64();
    let s = -(lo & 1);
    lo = (lo >>> 1 | (hi & 1) << 31) ^ s;
    hi = hi >>> 1 ^ s;
    return new PbLong(lo, hi);
  }
  /**
   * Read a `bool` field, a variant.
   */
  bool() {
    let [lo, hi] = this.varint64();
    return lo !== 0 || hi !== 0;
  }
  /**
   * Read a `fixed32` field, an unsigned, fixed-length 32-bit integer.
   */
  fixed32() {
    return this.view.getUint32((this.pos += 4) - 4, true);
  }
  /**
   * Read a `sfixed32` field, a signed, fixed-length 32-bit integer.
   */
  sfixed32() {
    return this.view.getInt32((this.pos += 4) - 4, true);
  }
  /**
   * Read a `fixed64` field, an unsigned, fixed-length 64 bit integer.
   */
  fixed64() {
    return new PbULong(this.sfixed32(), this.sfixed32());
  }
  /**
   * Read a `fixed64` field, a signed, fixed-length 64-bit integer.
   */
  sfixed64() {
    return new PbLong(this.sfixed32(), this.sfixed32());
  }
  /**
   * Read a `float` field, 32-bit floating point number.
   */
  float() {
    return this.view.getFloat32((this.pos += 4) - 4, true);
  }
  /**
   * Read a `double` field, a 64-bit floating point number.
   */
  double() {
    return this.view.getFloat64((this.pos += 8) - 8, true);
  }
  /**
   * Read a `bytes` field, length-delimited arbitrary data.
   */
  bytes() {
    let len = this.uint32();
    let start = this.pos;
    this.pos += len;
    this.assertBounds();
    return this.buf.subarray(start, start + len);
  }
  /**
   * Read a `string` field, length-delimited data converted to UTF-8 text.
   */
  string() {
    return this.textDecoder.decode(this.bytes());
  }
};

// node_modules/@protobuf-ts/runtime/build/es2015/assert.js
function assert(condition, msg) {
  if (!condition) {
    throw new Error(msg);
  }
}
var FLOAT32_MAX = 34028234663852886e22;
var FLOAT32_MIN = -34028234663852886e22;
var UINT32_MAX = 4294967295;
var INT32_MAX = 2147483647;
var INT32_MIN = -2147483648;
function assertInt32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid int 32: " + typeof arg);
  if (!Number.isInteger(arg) || arg > INT32_MAX || arg < INT32_MIN)
    throw new Error("invalid int 32: " + arg);
}
function assertUInt32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid uint 32: " + typeof arg);
  if (!Number.isInteger(arg) || arg > UINT32_MAX || arg < 0)
    throw new Error("invalid uint 32: " + arg);
}
function assertFloat32(arg) {
  if (typeof arg !== "number")
    throw new Error("invalid float 32: " + typeof arg);
  if (!Number.isFinite(arg))
    return;
  if (arg > FLOAT32_MAX || arg < FLOAT32_MIN)
    throw new Error("invalid float 32: " + arg);
}

// node_modules/@protobuf-ts/runtime/build/es2015/binary-writer.js
var defaultsWrite = {
  writeUnknownFields: true,
  writerFactory: () => new BinaryWriter()
};
function binaryWriteOptions(options) {
  return options ? Object.assign(Object.assign({}, defaultsWrite), options) : defaultsWrite;
}
var BinaryWriter = class {
  constructor(textEncoder) {
    this.stack = [];
    this.textEncoder = textEncoder !== null && textEncoder !== void 0 ? textEncoder : new TextEncoder();
    this.chunks = [];
    this.buf = [];
  }
  /**
   * Return all bytes written and reset this writer.
   */
  finish() {
    this.chunks.push(new Uint8Array(this.buf));
    let len = 0;
    for (let i = 0; i < this.chunks.length; i++)
      len += this.chunks[i].length;
    let bytes = new Uint8Array(len);
    let offset = 0;
    for (let i = 0; i < this.chunks.length; i++) {
      bytes.set(this.chunks[i], offset);
      offset += this.chunks[i].length;
    }
    this.chunks = [];
    return bytes;
  }
  /**
   * Start a new fork for length-delimited data like a message
   * or a packed repeated field.
   *
   * Must be joined later with `join()`.
   */
  fork() {
    this.stack.push({ chunks: this.chunks, buf: this.buf });
    this.chunks = [];
    this.buf = [];
    return this;
  }
  /**
   * Join the last fork. Write its length and bytes, then
   * return to the previous state.
   */
  join() {
    let chunk = this.finish();
    let prev = this.stack.pop();
    if (!prev)
      throw new Error("invalid state, fork stack empty");
    this.chunks = prev.chunks;
    this.buf = prev.buf;
    this.uint32(chunk.byteLength);
    return this.raw(chunk);
  }
  /**
   * Writes a tag (field number and wire type).
   *
   * Equivalent to `uint32( (fieldNo << 3 | type) >>> 0 )`.
   *
   * Generated code should compute the tag ahead of time and call `uint32()`.
   */
  tag(fieldNo, type) {
    return this.uint32((fieldNo << 3 | type) >>> 0);
  }
  /**
   * Write a chunk of raw bytes.
   */
  raw(chunk) {
    if (this.buf.length) {
      this.chunks.push(new Uint8Array(this.buf));
      this.buf = [];
    }
    this.chunks.push(chunk);
    return this;
  }
  /**
   * Write a `uint32` value, an unsigned 32 bit varint.
   */
  uint32(value) {
    assertUInt32(value);
    while (value > 127) {
      this.buf.push(value & 127 | 128);
      value = value >>> 7;
    }
    this.buf.push(value);
    return this;
  }
  /**
   * Write a `int32` value, a signed 32 bit varint.
   */
  int32(value) {
    assertInt32(value);
    varint32write(value, this.buf);
    return this;
  }
  /**
   * Write a `bool` value, a variant.
   */
  bool(value) {
    this.buf.push(value ? 1 : 0);
    return this;
  }
  /**
   * Write a `bytes` value, length-delimited arbitrary data.
   */
  bytes(value) {
    this.uint32(value.byteLength);
    return this.raw(value);
  }
  /**
   * Write a `string` value, length-delimited data converted to UTF-8 text.
   */
  string(value) {
    let chunk = this.textEncoder.encode(value);
    this.uint32(chunk.byteLength);
    return this.raw(chunk);
  }
  /**
   * Write a `float` value, 32-bit floating point number.
   */
  float(value) {
    assertFloat32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setFloat32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `double` value, a 64-bit floating point number.
   */
  double(value) {
    let chunk = new Uint8Array(8);
    new DataView(chunk.buffer).setFloat64(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `fixed32` value, an unsigned, fixed-length 32-bit integer.
   */
  fixed32(value) {
    assertUInt32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setUint32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `sfixed32` value, a signed, fixed-length 32-bit integer.
   */
  sfixed32(value) {
    assertInt32(value);
    let chunk = new Uint8Array(4);
    new DataView(chunk.buffer).setInt32(0, value, true);
    return this.raw(chunk);
  }
  /**
   * Write a `sint32` value, a signed, zigzag-encoded 32-bit varint.
   */
  sint32(value) {
    assertInt32(value);
    value = (value << 1 ^ value >> 31) >>> 0;
    varint32write(value, this.buf);
    return this;
  }
  /**
   * Write a `fixed64` value, a signed, fixed-length 64-bit integer.
   */
  sfixed64(value) {
    let chunk = new Uint8Array(8);
    let view = new DataView(chunk.buffer);
    let long = PbLong.from(value);
    view.setInt32(0, long.lo, true);
    view.setInt32(4, long.hi, true);
    return this.raw(chunk);
  }
  /**
   * Write a `fixed64` value, an unsigned, fixed-length 64 bit integer.
   */
  fixed64(value) {
    let chunk = new Uint8Array(8);
    let view = new DataView(chunk.buffer);
    let long = PbULong.from(value);
    view.setInt32(0, long.lo, true);
    view.setInt32(4, long.hi, true);
    return this.raw(chunk);
  }
  /**
   * Write a `int64` value, a signed 64-bit varint.
   */
  int64(value) {
    let long = PbLong.from(value);
    varint64write(long.lo, long.hi, this.buf);
    return this;
  }
  /**
   * Write a `sint64` value, a signed, zig-zag-encoded 64-bit varint.
   */
  sint64(value) {
    let long = PbLong.from(value), sign = long.hi >> 31, lo = long.lo << 1 ^ sign, hi = (long.hi << 1 | long.lo >>> 31) ^ sign;
    varint64write(lo, hi, this.buf);
    return this;
  }
  /**
   * Write a `uint64` value, an unsigned 64-bit varint.
   */
  uint64(value) {
    let long = PbULong.from(value);
    varint64write(long.lo, long.hi, this.buf);
    return this;
  }
};

// node_modules/@protobuf-ts/runtime/build/es2015/json-format-contract.js
var defaultsWrite2 = {
  emitDefaultValues: false,
  enumAsInteger: false,
  useProtoFieldName: false,
  prettySpaces: 0
};
var defaultsRead2 = {
  ignoreUnknownFields: false
};
function jsonReadOptions(options) {
  return options ? Object.assign(Object.assign({}, defaultsRead2), options) : defaultsRead2;
}
function jsonWriteOptions(options) {
  return options ? Object.assign(Object.assign({}, defaultsWrite2), options) : defaultsWrite2;
}

// node_modules/@protobuf-ts/runtime/build/es2015/message-type-contract.js
var MESSAGE_TYPE = Symbol.for("protobuf-ts/message-type");

// node_modules/@protobuf-ts/runtime/build/es2015/lower-camel-case.js
function lowerCamelCase(snakeCase) {
  let capNext = false;
  const sb = [];
  for (let i = 0; i < snakeCase.length; i++) {
    let next = snakeCase.charAt(i);
    if (next == "_") {
      capNext = true;
    } else if (/\d/.test(next)) {
      sb.push(next);
      capNext = true;
    } else if (capNext) {
      sb.push(next.toUpperCase());
      capNext = false;
    } else if (i == 0) {
      sb.push(next.toLowerCase());
    } else {
      sb.push(next);
    }
  }
  return sb.join("");
}

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-info.js
var ScalarType;
(function(ScalarType2) {
  ScalarType2[ScalarType2["DOUBLE"] = 1] = "DOUBLE";
  ScalarType2[ScalarType2["FLOAT"] = 2] = "FLOAT";
  ScalarType2[ScalarType2["INT64"] = 3] = "INT64";
  ScalarType2[ScalarType2["UINT64"] = 4] = "UINT64";
  ScalarType2[ScalarType2["INT32"] = 5] = "INT32";
  ScalarType2[ScalarType2["FIXED64"] = 6] = "FIXED64";
  ScalarType2[ScalarType2["FIXED32"] = 7] = "FIXED32";
  ScalarType2[ScalarType2["BOOL"] = 8] = "BOOL";
  ScalarType2[ScalarType2["STRING"] = 9] = "STRING";
  ScalarType2[ScalarType2["BYTES"] = 12] = "BYTES";
  ScalarType2[ScalarType2["UINT32"] = 13] = "UINT32";
  ScalarType2[ScalarType2["SFIXED32"] = 15] = "SFIXED32";
  ScalarType2[ScalarType2["SFIXED64"] = 16] = "SFIXED64";
  ScalarType2[ScalarType2["SINT32"] = 17] = "SINT32";
  ScalarType2[ScalarType2["SINT64"] = 18] = "SINT64";
})(ScalarType || (ScalarType = {}));
var LongType;
(function(LongType2) {
  LongType2[LongType2["BIGINT"] = 0] = "BIGINT";
  LongType2[LongType2["STRING"] = 1] = "STRING";
  LongType2[LongType2["NUMBER"] = 2] = "NUMBER";
})(LongType || (LongType = {}));
var RepeatType;
(function(RepeatType2) {
  RepeatType2[RepeatType2["NO"] = 0] = "NO";
  RepeatType2[RepeatType2["PACKED"] = 1] = "PACKED";
  RepeatType2[RepeatType2["UNPACKED"] = 2] = "UNPACKED";
})(RepeatType || (RepeatType = {}));
function normalizeFieldInfo(field) {
  var _a, _b, _c, _d;
  field.localName = (_a = field.localName) !== null && _a !== void 0 ? _a : lowerCamelCase(field.name);
  field.jsonName = (_b = field.jsonName) !== null && _b !== void 0 ? _b : lowerCamelCase(field.name);
  field.repeat = (_c = field.repeat) !== null && _c !== void 0 ? _c : RepeatType.NO;
  field.opt = (_d = field.opt) !== null && _d !== void 0 ? _d : field.repeat ? false : field.oneof ? false : field.kind == "message";
  return field;
}

// node_modules/@protobuf-ts/runtime/build/es2015/oneof.js
function isOneofGroup(any) {
  if (typeof any != "object" || any === null || !any.hasOwnProperty("oneofKind")) {
    return false;
  }
  switch (typeof any.oneofKind) {
    case "string":
      if (any[any.oneofKind] === void 0)
        return false;
      return Object.keys(any).length == 2;
    case "undefined":
      return Object.keys(any).length == 1;
    default:
      return false;
  }
}

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-type-check.js
var ReflectionTypeCheck = class {
  constructor(info) {
    var _a;
    this.fields = (_a = info.fields) !== null && _a !== void 0 ? _a : [];
  }
  prepare() {
    if (this.data)
      return;
    const req = [], known = [], oneofs = [];
    for (let field of this.fields) {
      if (field.oneof) {
        if (!oneofs.includes(field.oneof)) {
          oneofs.push(field.oneof);
          req.push(field.oneof);
          known.push(field.oneof);
        }
      } else {
        known.push(field.localName);
        switch (field.kind) {
          case "scalar":
          case "enum":
            if (!field.opt || field.repeat)
              req.push(field.localName);
            break;
          case "message":
            if (field.repeat)
              req.push(field.localName);
            break;
          case "map":
            req.push(field.localName);
            break;
        }
      }
    }
    this.data = { req, known, oneofs: Object.values(oneofs) };
  }
  /**
   * Is the argument a valid message as specified by the
   * reflection information?
   *
   * Checks all field types recursively. The `depth`
   * specifies how deep into the structure the check will be.
   *
   * With a depth of 0, only the presence of fields
   * is checked.
   *
   * With a depth of 1 or more, the field types are checked.
   *
   * With a depth of 2 or more, the members of map, repeated
   * and message fields are checked.
   *
   * Message fields will be checked recursively with depth - 1.
   *
   * The number of map entries / repeated values being checked
   * is < depth.
   */
  is(message, depth, allowExcessProperties = false) {
    if (depth < 0)
      return true;
    if (message === null || message === void 0 || typeof message != "object")
      return false;
    this.prepare();
    let keys = Object.keys(message), data = this.data;
    if (keys.length < data.req.length || data.req.some((n) => !keys.includes(n)))
      return false;
    if (!allowExcessProperties) {
      if (keys.some((k) => !data.known.includes(k)))
        return false;
    }
    if (depth < 1) {
      return true;
    }
    for (const name of data.oneofs) {
      const group = message[name];
      if (!isOneofGroup(group))
        return false;
      if (group.oneofKind === void 0)
        continue;
      const field = this.fields.find((f) => f.localName === group.oneofKind);
      if (!field)
        return false;
      if (!this.field(group[group.oneofKind], field, allowExcessProperties, depth))
        return false;
    }
    for (const field of this.fields) {
      if (field.oneof !== void 0)
        continue;
      if (!this.field(message[field.localName], field, allowExcessProperties, depth))
        return false;
    }
    return true;
  }
  field(arg, field, allowExcessProperties, depth) {
    let repeated = field.repeat;
    switch (field.kind) {
      case "scalar":
        if (arg === void 0)
          return field.opt;
        if (repeated)
          return this.scalars(arg, field.T, depth, field.L);
        return this.scalar(arg, field.T, field.L);
      case "enum":
        if (arg === void 0)
          return field.opt;
        if (repeated)
          return this.scalars(arg, ScalarType.INT32, depth);
        return this.scalar(arg, ScalarType.INT32);
      case "message":
        if (arg === void 0)
          return true;
        if (repeated)
          return this.messages(arg, field.T(), allowExcessProperties, depth);
        return this.message(arg, field.T(), allowExcessProperties, depth);
      case "map":
        if (typeof arg != "object" || arg === null)
          return false;
        if (depth < 2)
          return true;
        if (!this.mapKeys(arg, field.K, depth))
          return false;
        switch (field.V.kind) {
          case "scalar":
            return this.scalars(Object.values(arg), field.V.T, depth, field.V.L);
          case "enum":
            return this.scalars(Object.values(arg), ScalarType.INT32, depth);
          case "message":
            return this.messages(Object.values(arg), field.V.T(), allowExcessProperties, depth);
        }
        break;
    }
    return true;
  }
  message(arg, type, allowExcessProperties, depth) {
    if (allowExcessProperties) {
      return type.isAssignable(arg, depth);
    }
    return type.is(arg, depth);
  }
  messages(arg, type, allowExcessProperties, depth) {
    if (!Array.isArray(arg))
      return false;
    if (depth < 2)
      return true;
    if (allowExcessProperties) {
      for (let i = 0; i < arg.length && i < depth; i++)
        if (!type.isAssignable(arg[i], depth - 1))
          return false;
    } else {
      for (let i = 0; i < arg.length && i < depth; i++)
        if (!type.is(arg[i], depth - 1))
          return false;
    }
    return true;
  }
  scalar(arg, type, longType) {
    let argType = typeof arg;
    switch (type) {
      case ScalarType.UINT64:
      case ScalarType.FIXED64:
      case ScalarType.INT64:
      case ScalarType.SFIXED64:
      case ScalarType.SINT64:
        switch (longType) {
          case LongType.BIGINT:
            return argType == "bigint";
          case LongType.NUMBER:
            return argType == "number" && !isNaN(arg);
          default:
            return argType == "string";
        }
      case ScalarType.BOOL:
        return argType == "boolean";
      case ScalarType.STRING:
        return argType == "string";
      case ScalarType.BYTES:
        return arg instanceof Uint8Array;
      case ScalarType.DOUBLE:
      case ScalarType.FLOAT:
        return argType == "number" && !isNaN(arg);
      default:
        return argType == "number" && Number.isInteger(arg);
    }
  }
  scalars(arg, type, depth, longType) {
    if (!Array.isArray(arg))
      return false;
    if (depth < 2)
      return true;
    if (Array.isArray(arg)) {
      for (let i = 0; i < arg.length && i < depth; i++)
        if (!this.scalar(arg[i], type, longType))
          return false;
    }
    return true;
  }
  mapKeys(map, type, depth) {
    let keys = Object.keys(map);
    switch (type) {
      case ScalarType.INT32:
      case ScalarType.FIXED32:
      case ScalarType.SFIXED32:
      case ScalarType.SINT32:
      case ScalarType.UINT32:
        return this.scalars(keys.slice(0, depth).map((k) => parseInt(k)), type, depth);
      case ScalarType.BOOL:
        return this.scalars(keys.slice(0, depth).map((k) => k == "true" ? true : k == "false" ? false : k), type, depth);
      default:
        return this.scalars(keys, type, depth, LongType.STRING);
    }
  }
};

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-long-convert.js
function reflectionLongConvert(long, type) {
  switch (type) {
    case LongType.BIGINT:
      return long.toBigInt();
    case LongType.NUMBER:
      return long.toNumber();
    default:
      return long.toString();
  }
}

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-json-reader.js
var ReflectionJsonReader = class {
  constructor(info) {
    this.info = info;
  }
  prepare() {
    var _a;
    if (this.fMap === void 0) {
      this.fMap = {};
      const fieldsInput = (_a = this.info.fields) !== null && _a !== void 0 ? _a : [];
      for (const field of fieldsInput) {
        this.fMap[field.name] = field;
        this.fMap[field.jsonName] = field;
        this.fMap[field.localName] = field;
      }
    }
  }
  // Cannot parse JSON <type of jsonValue> for <type name>#<fieldName>.
  assert(condition, fieldName, jsonValue) {
    if (!condition) {
      let what = typeofJsonValue(jsonValue);
      if (what == "number" || what == "boolean")
        what = jsonValue.toString();
      throw new Error(`Cannot parse JSON ${what} for ${this.info.typeName}#${fieldName}`);
    }
  }
  /**
   * Reads a message from canonical JSON format into the target message.
   *
   * Repeated fields are appended. Map entries are added, overwriting
   * existing keys.
   *
   * If a message field is already present, it will be merged with the
   * new data.
   */
  read(input, message, options) {
    this.prepare();
    const oneofsHandled = [];
    for (const [jsonKey, jsonValue] of Object.entries(input)) {
      const field = this.fMap[jsonKey];
      if (!field) {
        if (!options.ignoreUnknownFields)
          throw new Error(`Found unknown field while reading ${this.info.typeName} from JSON format. JSON key: ${jsonKey}`);
        continue;
      }
      const localName = field.localName;
      let target;
      if (field.oneof) {
        if (jsonValue === null && (field.kind !== "enum" || field.T()[0] !== "google.protobuf.NullValue")) {
          continue;
        }
        if (oneofsHandled.includes(field.oneof))
          throw new Error(`Multiple members of the oneof group "${field.oneof}" of ${this.info.typeName} are present in JSON.`);
        oneofsHandled.push(field.oneof);
        target = message[field.oneof] = {
          oneofKind: localName
        };
      } else {
        target = message;
      }
      if (field.kind == "map") {
        if (jsonValue === null) {
          continue;
        }
        this.assert(isJsonObject(jsonValue), field.name, jsonValue);
        const fieldObj = target[localName];
        for (const [jsonObjKey, jsonObjValue] of Object.entries(jsonValue)) {
          this.assert(jsonObjValue !== null, field.name + " map value", null);
          let val;
          switch (field.V.kind) {
            case "message":
              val = field.V.T().internalJsonRead(jsonObjValue, options);
              break;
            case "enum":
              val = this.enum(field.V.T(), jsonObjValue, field.name, options.ignoreUnknownFields);
              if (val === false)
                continue;
              break;
            case "scalar":
              val = this.scalar(jsonObjValue, field.V.T, field.V.L, field.name);
              break;
          }
          this.assert(val !== void 0, field.name + " map value", jsonObjValue);
          let key = jsonObjKey;
          if (field.K == ScalarType.BOOL)
            key = key == "true" ? true : key == "false" ? false : key;
          key = this.scalar(key, field.K, LongType.STRING, field.name).toString();
          fieldObj[key] = val;
        }
      } else if (field.repeat) {
        if (jsonValue === null)
          continue;
        this.assert(Array.isArray(jsonValue), field.name, jsonValue);
        const fieldArr = target[localName];
        for (const jsonItem of jsonValue) {
          this.assert(jsonItem !== null, field.name, null);
          let val;
          switch (field.kind) {
            case "message":
              val = field.T().internalJsonRead(jsonItem, options);
              break;
            case "enum":
              val = this.enum(field.T(), jsonItem, field.name, options.ignoreUnknownFields);
              if (val === false)
                continue;
              break;
            case "scalar":
              val = this.scalar(jsonItem, field.T, field.L, field.name);
              break;
          }
          this.assert(val !== void 0, field.name, jsonValue);
          fieldArr.push(val);
        }
      } else {
        switch (field.kind) {
          case "message":
            if (jsonValue === null && field.T().typeName != "google.protobuf.Value") {
              this.assert(field.oneof === void 0, field.name + " (oneof member)", null);
              continue;
            }
            target[localName] = field.T().internalJsonRead(jsonValue, options, target[localName]);
            break;
          case "enum":
            if (jsonValue === null)
              continue;
            let val = this.enum(field.T(), jsonValue, field.name, options.ignoreUnknownFields);
            if (val === false)
              continue;
            target[localName] = val;
            break;
          case "scalar":
            if (jsonValue === null)
              continue;
            target[localName] = this.scalar(jsonValue, field.T, field.L, field.name);
            break;
        }
      }
    }
  }
  /**
   * Returns `false` for unrecognized string representations.
   *
   * google.protobuf.NullValue accepts only JSON `null` (or the old `"NULL_VALUE"`).
   */
  enum(type, json, fieldName, ignoreUnknownFields) {
    if (type[0] == "google.protobuf.NullValue")
      assert(json === null || json === "NULL_VALUE", `Unable to parse field ${this.info.typeName}#${fieldName}, enum ${type[0]} only accepts null.`);
    if (json === null)
      return 0;
    switch (typeof json) {
      case "number":
        assert(Number.isInteger(json), `Unable to parse field ${this.info.typeName}#${fieldName}, enum can only be integral number, got ${json}.`);
        return json;
      case "string":
        let localEnumName = json;
        if (type[2] && json.substring(0, type[2].length) === type[2])
          localEnumName = json.substring(type[2].length);
        let enumNumber = type[1][localEnumName];
        if (typeof enumNumber === "undefined" && ignoreUnknownFields) {
          return false;
        }
        assert(typeof enumNumber == "number", `Unable to parse field ${this.info.typeName}#${fieldName}, enum ${type[0]} has no value for "${json}".`);
        return enumNumber;
    }
    assert(false, `Unable to parse field ${this.info.typeName}#${fieldName}, cannot parse enum value from ${typeof json}".`);
  }
  scalar(json, type, longType, fieldName) {
    let e;
    try {
      switch (type) {
        // float, double: JSON value will be a number or one of the special string values "NaN", "Infinity", and "-Infinity".
        // Either numbers or strings are accepted. Exponent notation is also accepted.
        case ScalarType.DOUBLE:
        case ScalarType.FLOAT:
          if (json === null)
            return 0;
          if (json === "NaN")
            return Number.NaN;
          if (json === "Infinity")
            return Number.POSITIVE_INFINITY;
          if (json === "-Infinity")
            return Number.NEGATIVE_INFINITY;
          if (json === "") {
            e = "empty string";
            break;
          }
          if (typeof json == "string" && json.trim().length !== json.length) {
            e = "extra whitespace";
            break;
          }
          if (typeof json != "string" && typeof json != "number") {
            break;
          }
          let float = Number(json);
          if (Number.isNaN(float)) {
            e = "not a number";
            break;
          }
          if (!Number.isFinite(float)) {
            e = "too large or small";
            break;
          }
          if (type == ScalarType.FLOAT)
            assertFloat32(float);
          return float;
        // int32, fixed32, uint32: JSON value will be a decimal number. Either numbers or strings are accepted.
        case ScalarType.INT32:
        case ScalarType.FIXED32:
        case ScalarType.SFIXED32:
        case ScalarType.SINT32:
        case ScalarType.UINT32:
          if (json === null)
            return 0;
          let int32;
          if (typeof json == "number")
            int32 = json;
          else if (json === "")
            e = "empty string";
          else if (typeof json == "string") {
            if (json.trim().length !== json.length)
              e = "extra whitespace";
            else
              int32 = Number(json);
          }
          if (int32 === void 0)
            break;
          if (type == ScalarType.UINT32)
            assertUInt32(int32);
          else
            assertInt32(int32);
          return int32;
        // int64, fixed64, uint64: JSON value will be a decimal string. Either numbers or strings are accepted.
        case ScalarType.INT64:
        case ScalarType.SFIXED64:
        case ScalarType.SINT64:
          if (json === null)
            return reflectionLongConvert(PbLong.ZERO, longType);
          if (typeof json != "number" && typeof json != "string")
            break;
          return reflectionLongConvert(PbLong.from(json), longType);
        case ScalarType.FIXED64:
        case ScalarType.UINT64:
          if (json === null)
            return reflectionLongConvert(PbULong.ZERO, longType);
          if (typeof json != "number" && typeof json != "string")
            break;
          return reflectionLongConvert(PbULong.from(json), longType);
        // bool:
        case ScalarType.BOOL:
          if (json === null)
            return false;
          if (typeof json !== "boolean")
            break;
          return json;
        // string:
        case ScalarType.STRING:
          if (json === null)
            return "";
          if (typeof json !== "string") {
            e = "extra whitespace";
            break;
          }
          try {
            encodeURIComponent(json);
          } catch (e2) {
            e2 = "invalid UTF8";
            break;
          }
          return json;
        // bytes: JSON value will be the data encoded as a string using standard base64 encoding with paddings.
        // Either standard or URL-safe base64 encoding with/without paddings are accepted.
        case ScalarType.BYTES:
          if (json === null || json === "")
            return new Uint8Array(0);
          if (typeof json !== "string")
            break;
          return base64decode(json);
      }
    } catch (error) {
      e = error.message;
    }
    this.assert(false, fieldName + (e ? " - " + e : ""), json);
  }
};

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-json-writer.js
var ReflectionJsonWriter = class {
  constructor(info) {
    var _a;
    this.fields = (_a = info.fields) !== null && _a !== void 0 ? _a : [];
  }
  /**
   * Converts the message to a JSON object, based on the field descriptors.
   */
  write(message, options) {
    const json = {}, source = message;
    for (const field of this.fields) {
      if (!field.oneof) {
        let jsonValue2 = this.field(field, source[field.localName], options);
        if (jsonValue2 !== void 0)
          json[options.useProtoFieldName ? field.name : field.jsonName] = jsonValue2;
        continue;
      }
      const group = source[field.oneof];
      if (group.oneofKind !== field.localName)
        continue;
      const opt = field.kind == "scalar" || field.kind == "enum" ? Object.assign(Object.assign({}, options), { emitDefaultValues: true }) : options;
      let jsonValue = this.field(field, group[field.localName], opt);
      assert(jsonValue !== void 0);
      json[options.useProtoFieldName ? field.name : field.jsonName] = jsonValue;
    }
    return json;
  }
  field(field, value, options) {
    let jsonValue = void 0;
    if (field.kind == "map") {
      assert(typeof value == "object" && value !== null);
      const jsonObj = {};
      switch (field.V.kind) {
        case "scalar":
          for (const [entryKey, entryValue] of Object.entries(value)) {
            const val = this.scalar(field.V.T, entryValue, field.name, false, true);
            assert(val !== void 0);
            jsonObj[entryKey.toString()] = val;
          }
          break;
        case "message":
          const messageType = field.V.T();
          for (const [entryKey, entryValue] of Object.entries(value)) {
            const val = this.message(messageType, entryValue, field.name, options);
            assert(val !== void 0);
            jsonObj[entryKey.toString()] = val;
          }
          break;
        case "enum":
          const enumInfo = field.V.T();
          for (const [entryKey, entryValue] of Object.entries(value)) {
            assert(entryValue === void 0 || typeof entryValue == "number");
            const val = this.enum(enumInfo, entryValue, field.name, false, true, options.enumAsInteger);
            assert(val !== void 0);
            jsonObj[entryKey.toString()] = val;
          }
          break;
      }
      if (options.emitDefaultValues || Object.keys(jsonObj).length > 0)
        jsonValue = jsonObj;
    } else if (field.repeat) {
      assert(Array.isArray(value));
      const jsonArr = [];
      switch (field.kind) {
        case "scalar":
          for (let i = 0; i < value.length; i++) {
            const val = this.scalar(field.T, value[i], field.name, field.opt, true);
            assert(val !== void 0);
            jsonArr.push(val);
          }
          break;
        case "enum":
          const enumInfo = field.T();
          for (let i = 0; i < value.length; i++) {
            assert(value[i] === void 0 || typeof value[i] == "number");
            const val = this.enum(enumInfo, value[i], field.name, field.opt, true, options.enumAsInteger);
            assert(val !== void 0);
            jsonArr.push(val);
          }
          break;
        case "message":
          const messageType = field.T();
          for (let i = 0; i < value.length; i++) {
            const val = this.message(messageType, value[i], field.name, options);
            assert(val !== void 0);
            jsonArr.push(val);
          }
          break;
      }
      if (options.emitDefaultValues || jsonArr.length > 0 || options.emitDefaultValues)
        jsonValue = jsonArr;
    } else {
      switch (field.kind) {
        case "scalar":
          jsonValue = this.scalar(field.T, value, field.name, field.opt, options.emitDefaultValues);
          break;
        case "enum":
          jsonValue = this.enum(field.T(), value, field.name, field.opt, options.emitDefaultValues, options.enumAsInteger);
          break;
        case "message":
          jsonValue = this.message(field.T(), value, field.name, options);
          break;
      }
    }
    return jsonValue;
  }
  /**
   * Returns `null` as the default for google.protobuf.NullValue.
   */
  enum(type, value, fieldName, optional, emitDefaultValues, enumAsInteger) {
    if (type[0] == "google.protobuf.NullValue")
      return !emitDefaultValues && !optional ? void 0 : null;
    if (value === void 0) {
      assert(optional);
      return void 0;
    }
    if (value === 0 && !emitDefaultValues && !optional)
      return void 0;
    assert(typeof value == "number");
    assert(Number.isInteger(value));
    if (enumAsInteger || !type[1].hasOwnProperty(value))
      return value;
    if (type[2])
      return type[2] + type[1][value];
    return type[1][value];
  }
  message(type, value, fieldName, options) {
    if (value === void 0)
      return options.emitDefaultValues ? null : void 0;
    return type.internalJsonWrite(value, options);
  }
  scalar(type, value, fieldName, optional, emitDefaultValues) {
    if (value === void 0) {
      assert(optional);
      return void 0;
    }
    const ed = emitDefaultValues || optional;
    switch (type) {
      // int32, fixed32, uint32: JSON value will be a decimal number. Either numbers or strings are accepted.
      case ScalarType.INT32:
      case ScalarType.SFIXED32:
      case ScalarType.SINT32:
        if (value === 0)
          return ed ? 0 : void 0;
        assertInt32(value);
        return value;
      case ScalarType.FIXED32:
      case ScalarType.UINT32:
        if (value === 0)
          return ed ? 0 : void 0;
        assertUInt32(value);
        return value;
      // float, double: JSON value will be a number or one of the special string values "NaN", "Infinity", and "-Infinity".
      // Either numbers or strings are accepted. Exponent notation is also accepted.
      case ScalarType.FLOAT:
        assertFloat32(value);
      case ScalarType.DOUBLE:
        if (value === 0)
          return ed ? 0 : void 0;
        assert(typeof value == "number");
        if (Number.isNaN(value))
          return "NaN";
        if (value === Number.POSITIVE_INFINITY)
          return "Infinity";
        if (value === Number.NEGATIVE_INFINITY)
          return "-Infinity";
        return value;
      // string:
      case ScalarType.STRING:
        if (value === "")
          return ed ? "" : void 0;
        assert(typeof value == "string");
        return value;
      // bool:
      case ScalarType.BOOL:
        if (value === false)
          return ed ? false : void 0;
        assert(typeof value == "boolean");
        return value;
      // JSON value will be a decimal string. Either numbers or strings are accepted.
      case ScalarType.UINT64:
      case ScalarType.FIXED64:
        assert(typeof value == "number" || typeof value == "string" || typeof value == "bigint");
        let ulong = PbULong.from(value);
        if (ulong.isZero() && !ed)
          return void 0;
        return ulong.toString();
      // JSON value will be a decimal string. Either numbers or strings are accepted.
      case ScalarType.INT64:
      case ScalarType.SFIXED64:
      case ScalarType.SINT64:
        assert(typeof value == "number" || typeof value == "string" || typeof value == "bigint");
        let long = PbLong.from(value);
        if (long.isZero() && !ed)
          return void 0;
        return long.toString();
      // bytes: JSON value will be the data encoded as a string using standard base64 encoding with paddings.
      // Either standard or URL-safe base64 encoding with/without paddings are accepted.
      case ScalarType.BYTES:
        assert(value instanceof Uint8Array);
        if (!value.byteLength)
          return ed ? "" : void 0;
        return base64encode(value);
    }
  }
};

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-scalar-default.js
function reflectionScalarDefault(type, longType = LongType.STRING) {
  switch (type) {
    case ScalarType.BOOL:
      return false;
    case ScalarType.UINT64:
    case ScalarType.FIXED64:
      return reflectionLongConvert(PbULong.ZERO, longType);
    case ScalarType.INT64:
    case ScalarType.SFIXED64:
    case ScalarType.SINT64:
      return reflectionLongConvert(PbLong.ZERO, longType);
    case ScalarType.DOUBLE:
    case ScalarType.FLOAT:
      return 0;
    case ScalarType.BYTES:
      return new Uint8Array(0);
    case ScalarType.STRING:
      return "";
    default:
      return 0;
  }
}

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-binary-reader.js
var ReflectionBinaryReader = class {
  constructor(info) {
    this.info = info;
  }
  prepare() {
    var _a;
    if (!this.fieldNoToField) {
      const fieldsInput = (_a = this.info.fields) !== null && _a !== void 0 ? _a : [];
      this.fieldNoToField = new Map(fieldsInput.map((field) => [field.no, field]));
    }
  }
  /**
   * Reads a message from binary format into the target message.
   *
   * Repeated fields are appended. Map entries are added, overwriting
   * existing keys.
   *
   * If a message field is already present, it will be merged with the
   * new data.
   */
  read(reader, message, options, length) {
    this.prepare();
    const end = length === void 0 ? reader.len : reader.pos + length;
    while (reader.pos < end) {
      const [fieldNo, wireType] = reader.tag(), field = this.fieldNoToField.get(fieldNo);
      if (!field) {
        let u = options.readUnknownField;
        if (u == "throw")
          throw new Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.info.typeName}`);
        let d = reader.skip(wireType);
        if (u !== false)
          (u === true ? UnknownFieldHandler.onRead : u)(this.info.typeName, message, fieldNo, wireType, d);
        continue;
      }
      let target = message, repeated = field.repeat, localName = field.localName;
      if (field.oneof) {
        target = target[field.oneof];
        if (target.oneofKind !== localName)
          target = message[field.oneof] = {
            oneofKind: localName
          };
      }
      switch (field.kind) {
        case "scalar":
        case "enum":
          let T = field.kind == "enum" ? ScalarType.INT32 : field.T;
          let L = field.kind == "scalar" ? field.L : void 0;
          if (repeated) {
            let arr = target[localName];
            if (wireType == WireType.LengthDelimited && T != ScalarType.STRING && T != ScalarType.BYTES) {
              let e = reader.uint32() + reader.pos;
              while (reader.pos < e)
                arr.push(this.scalar(reader, T, L));
            } else
              arr.push(this.scalar(reader, T, L));
          } else
            target[localName] = this.scalar(reader, T, L);
          break;
        case "message":
          if (repeated) {
            let arr = target[localName];
            let msg = field.T().internalBinaryRead(reader, reader.uint32(), options);
            arr.push(msg);
          } else
            target[localName] = field.T().internalBinaryRead(reader, reader.uint32(), options, target[localName]);
          break;
        case "map":
          let [mapKey, mapVal] = this.mapEntry(field, reader, options);
          target[localName][mapKey] = mapVal;
          break;
      }
    }
  }
  /**
   * Read a map field, expecting key field = 1, value field = 2
   */
  mapEntry(field, reader, options) {
    let length = reader.uint32();
    let end = reader.pos + length;
    let key = void 0;
    let val = void 0;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case 1:
          if (field.K == ScalarType.BOOL)
            key = reader.bool().toString();
          else
            key = this.scalar(reader, field.K, LongType.STRING);
          break;
        case 2:
          switch (field.V.kind) {
            case "scalar":
              val = this.scalar(reader, field.V.T, field.V.L);
              break;
            case "enum":
              val = reader.int32();
              break;
            case "message":
              val = field.V.T().internalBinaryRead(reader, reader.uint32(), options);
              break;
          }
          break;
        default:
          throw new Error(`Unknown field ${fieldNo} (wire type ${wireType}) in map entry for ${this.info.typeName}#${field.name}`);
      }
    }
    if (key === void 0) {
      let keyRaw = reflectionScalarDefault(field.K);
      key = field.K == ScalarType.BOOL ? keyRaw.toString() : keyRaw;
    }
    if (val === void 0)
      switch (field.V.kind) {
        case "scalar":
          val = reflectionScalarDefault(field.V.T, field.V.L);
          break;
        case "enum":
          val = 0;
          break;
        case "message":
          val = field.V.T().create();
          break;
      }
    return [key, val];
  }
  scalar(reader, type, longType) {
    switch (type) {
      case ScalarType.INT32:
        return reader.int32();
      case ScalarType.STRING:
        return reader.string();
      case ScalarType.BOOL:
        return reader.bool();
      case ScalarType.DOUBLE:
        return reader.double();
      case ScalarType.FLOAT:
        return reader.float();
      case ScalarType.INT64:
        return reflectionLongConvert(reader.int64(), longType);
      case ScalarType.UINT64:
        return reflectionLongConvert(reader.uint64(), longType);
      case ScalarType.FIXED64:
        return reflectionLongConvert(reader.fixed64(), longType);
      case ScalarType.FIXED32:
        return reader.fixed32();
      case ScalarType.BYTES:
        return reader.bytes();
      case ScalarType.UINT32:
        return reader.uint32();
      case ScalarType.SFIXED32:
        return reader.sfixed32();
      case ScalarType.SFIXED64:
        return reflectionLongConvert(reader.sfixed64(), longType);
      case ScalarType.SINT32:
        return reader.sint32();
      case ScalarType.SINT64:
        return reflectionLongConvert(reader.sint64(), longType);
    }
  }
};

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-binary-writer.js
var ReflectionBinaryWriter = class {
  constructor(info) {
    this.info = info;
  }
  prepare() {
    if (!this.fields) {
      const fieldsInput = this.info.fields ? this.info.fields.concat() : [];
      this.fields = fieldsInput.sort((a, b) => a.no - b.no);
    }
  }
  /**
   * Writes the message to binary format.
   */
  write(message, writer, options) {
    this.prepare();
    for (const field of this.fields) {
      let value, emitDefault, repeated = field.repeat, localName = field.localName;
      if (field.oneof) {
        const group = message[field.oneof];
        if (group.oneofKind !== localName)
          continue;
        value = group[localName];
        emitDefault = true;
      } else {
        value = message[localName];
        emitDefault = false;
      }
      switch (field.kind) {
        case "scalar":
        case "enum":
          let T = field.kind == "enum" ? ScalarType.INT32 : field.T;
          if (repeated) {
            assert(Array.isArray(value));
            if (repeated == RepeatType.PACKED)
              this.packed(writer, T, field.no, value);
            else
              for (const item of value)
                this.scalar(writer, T, field.no, item, true);
          } else if (value === void 0)
            assert(field.opt);
          else
            this.scalar(writer, T, field.no, value, emitDefault || field.opt);
          break;
        case "message":
          if (repeated) {
            assert(Array.isArray(value));
            for (const item of value)
              this.message(writer, options, field.T(), field.no, item);
          } else {
            this.message(writer, options, field.T(), field.no, value);
          }
          break;
        case "map":
          assert(typeof value == "object" && value !== null);
          for (const [key, val] of Object.entries(value))
            this.mapEntry(writer, options, field, key, val);
          break;
      }
    }
    let u = options.writeUnknownFields;
    if (u !== false)
      (u === true ? UnknownFieldHandler.onWrite : u)(this.info.typeName, message, writer);
  }
  mapEntry(writer, options, field, key, value) {
    writer.tag(field.no, WireType.LengthDelimited);
    writer.fork();
    let keyValue = key;
    switch (field.K) {
      case ScalarType.INT32:
      case ScalarType.FIXED32:
      case ScalarType.UINT32:
      case ScalarType.SFIXED32:
      case ScalarType.SINT32:
        keyValue = Number.parseInt(key);
        break;
      case ScalarType.BOOL:
        assert(key == "true" || key == "false");
        keyValue = key == "true";
        break;
    }
    this.scalar(writer, field.K, 1, keyValue, true);
    switch (field.V.kind) {
      case "scalar":
        this.scalar(writer, field.V.T, 2, value, true);
        break;
      case "enum":
        this.scalar(writer, ScalarType.INT32, 2, value, true);
        break;
      case "message":
        this.message(writer, options, field.V.T(), 2, value);
        break;
    }
    writer.join();
  }
  message(writer, options, handler, fieldNo, value) {
    if (value === void 0)
      return;
    handler.internalBinaryWrite(value, writer.tag(fieldNo, WireType.LengthDelimited).fork(), options);
    writer.join();
  }
  /**
   * Write a single scalar value.
   */
  scalar(writer, type, fieldNo, value, emitDefault) {
    let [wireType, method, isDefault] = this.scalarInfo(type, value);
    if (!isDefault || emitDefault) {
      writer.tag(fieldNo, wireType);
      writer[method](value);
    }
  }
  /**
   * Write an array of scalar values in packed format.
   */
  packed(writer, type, fieldNo, value) {
    if (!value.length)
      return;
    assert(type !== ScalarType.BYTES && type !== ScalarType.STRING);
    writer.tag(fieldNo, WireType.LengthDelimited);
    writer.fork();
    let [, method] = this.scalarInfo(type);
    for (let i = 0; i < value.length; i++)
      writer[method](value[i]);
    writer.join();
  }
  /**
   * Get information for writing a scalar value.
   *
   * Returns tuple:
   * [0]: appropriate WireType
   * [1]: name of the appropriate method of IBinaryWriter
   * [2]: whether the given value is a default value
   *
   * If argument `value` is omitted, [2] is always false.
   */
  scalarInfo(type, value) {
    let t = WireType.Varint;
    let m;
    let i = value === void 0;
    let d = value === 0;
    switch (type) {
      case ScalarType.INT32:
        m = "int32";
        break;
      case ScalarType.STRING:
        d = i || !value.length;
        t = WireType.LengthDelimited;
        m = "string";
        break;
      case ScalarType.BOOL:
        d = value === false;
        m = "bool";
        break;
      case ScalarType.UINT32:
        m = "uint32";
        break;
      case ScalarType.DOUBLE:
        t = WireType.Bit64;
        m = "double";
        break;
      case ScalarType.FLOAT:
        t = WireType.Bit32;
        m = "float";
        break;
      case ScalarType.INT64:
        d = i || PbLong.from(value).isZero();
        m = "int64";
        break;
      case ScalarType.UINT64:
        d = i || PbULong.from(value).isZero();
        m = "uint64";
        break;
      case ScalarType.FIXED64:
        d = i || PbULong.from(value).isZero();
        t = WireType.Bit64;
        m = "fixed64";
        break;
      case ScalarType.BYTES:
        d = i || !value.byteLength;
        t = WireType.LengthDelimited;
        m = "bytes";
        break;
      case ScalarType.FIXED32:
        t = WireType.Bit32;
        m = "fixed32";
        break;
      case ScalarType.SFIXED32:
        t = WireType.Bit32;
        m = "sfixed32";
        break;
      case ScalarType.SFIXED64:
        d = i || PbLong.from(value).isZero();
        t = WireType.Bit64;
        m = "sfixed64";
        break;
      case ScalarType.SINT32:
        m = "sint32";
        break;
      case ScalarType.SINT64:
        d = i || PbLong.from(value).isZero();
        m = "sint64";
        break;
    }
    return [t, m, i || d];
  }
};

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-create.js
function reflectionCreate(type) {
  const msg = type.messagePrototype ? Object.create(type.messagePrototype) : Object.defineProperty({}, MESSAGE_TYPE, { value: type });
  for (let field of type.fields) {
    let name = field.localName;
    if (field.opt)
      continue;
    if (field.oneof)
      msg[field.oneof] = { oneofKind: void 0 };
    else if (field.repeat)
      msg[name] = [];
    else
      switch (field.kind) {
        case "scalar":
          msg[name] = reflectionScalarDefault(field.T, field.L);
          break;
        case "enum":
          msg[name] = 0;
          break;
        case "map":
          msg[name] = {};
          break;
      }
  }
  return msg;
}

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-merge-partial.js
function reflectionMergePartial(info, target, source) {
  let fieldValue, input = source, output;
  for (let field of info.fields) {
    let name = field.localName;
    if (field.oneof) {
      const group = input[field.oneof];
      if ((group === null || group === void 0 ? void 0 : group.oneofKind) == void 0) {
        continue;
      }
      fieldValue = group[name];
      output = target[field.oneof];
      output.oneofKind = group.oneofKind;
      if (fieldValue == void 0) {
        delete output[name];
        continue;
      }
    } else {
      fieldValue = input[name];
      output = target;
      if (fieldValue == void 0) {
        continue;
      }
    }
    if (field.repeat)
      output[name].length = fieldValue.length;
    switch (field.kind) {
      case "scalar":
      case "enum":
        if (field.repeat)
          for (let i = 0; i < fieldValue.length; i++)
            output[name][i] = fieldValue[i];
        else
          output[name] = fieldValue;
        break;
      case "message":
        let T = field.T();
        if (field.repeat)
          for (let i = 0; i < fieldValue.length; i++)
            output[name][i] = T.create(fieldValue[i]);
        else if (output[name] === void 0)
          output[name] = T.create(fieldValue);
        else
          T.mergePartial(output[name], fieldValue);
        break;
      case "map":
        switch (field.V.kind) {
          case "scalar":
          case "enum":
            Object.assign(output[name], fieldValue);
            break;
          case "message":
            let T2 = field.V.T();
            for (let k of Object.keys(fieldValue))
              output[name][k] = T2.create(fieldValue[k]);
            break;
        }
        break;
    }
  }
}

// node_modules/@protobuf-ts/runtime/build/es2015/reflection-equals.js
function reflectionEquals(info, a, b) {
  if (a === b)
    return true;
  if (!a || !b)
    return false;
  for (let field of info.fields) {
    let localName = field.localName;
    let val_a = field.oneof ? a[field.oneof][localName] : a[localName];
    let val_b = field.oneof ? b[field.oneof][localName] : b[localName];
    switch (field.kind) {
      case "enum":
      case "scalar":
        let t = field.kind == "enum" ? ScalarType.INT32 : field.T;
        if (!(field.repeat ? repeatedPrimitiveEq(t, val_a, val_b) : primitiveEq(t, val_a, val_b)))
          return false;
        break;
      case "map":
        if (!(field.V.kind == "message" ? repeatedMsgEq(field.V.T(), objectValues(val_a), objectValues(val_b)) : repeatedPrimitiveEq(field.V.kind == "enum" ? ScalarType.INT32 : field.V.T, objectValues(val_a), objectValues(val_b))))
          return false;
        break;
      case "message":
        let T = field.T();
        if (!(field.repeat ? repeatedMsgEq(T, val_a, val_b) : T.equals(val_a, val_b)))
          return false;
        break;
    }
  }
  return true;
}
var objectValues = Object.values;
function primitiveEq(type, a, b) {
  if (a === b)
    return true;
  if (type !== ScalarType.BYTES)
    return false;
  let ba = a;
  let bb = b;
  if (ba.length !== bb.length)
    return false;
  for (let i = 0; i < ba.length; i++)
    if (ba[i] != bb[i])
      return false;
  return true;
}
function repeatedPrimitiveEq(type, a, b) {
  if (a.length !== b.length)
    return false;
  for (let i = 0; i < a.length; i++)
    if (!primitiveEq(type, a[i], b[i]))
      return false;
  return true;
}
function repeatedMsgEq(type, a, b) {
  if (a.length !== b.length)
    return false;
  for (let i = 0; i < a.length; i++)
    if (!type.equals(a[i], b[i]))
      return false;
  return true;
}

// node_modules/@protobuf-ts/runtime/build/es2015/message-type.js
var baseDescriptors = Object.getOwnPropertyDescriptors(Object.getPrototypeOf({}));
var messageTypeDescriptor = baseDescriptors[MESSAGE_TYPE] = {};
var MessageType = class {
  constructor(name, fields, options) {
    this.defaultCheckDepth = 16;
    this.typeName = name;
    this.fields = fields.map(normalizeFieldInfo);
    this.options = options !== null && options !== void 0 ? options : {};
    messageTypeDescriptor.value = this;
    this.messagePrototype = Object.create(null, baseDescriptors);
    this.refTypeCheck = new ReflectionTypeCheck(this);
    this.refJsonReader = new ReflectionJsonReader(this);
    this.refJsonWriter = new ReflectionJsonWriter(this);
    this.refBinReader = new ReflectionBinaryReader(this);
    this.refBinWriter = new ReflectionBinaryWriter(this);
  }
  create(value) {
    let message = reflectionCreate(this);
    if (value !== void 0) {
      reflectionMergePartial(this, message, value);
    }
    return message;
  }
  /**
   * Clone the message.
   *
   * Unknown fields are discarded.
   */
  clone(message) {
    let copy = this.create();
    reflectionMergePartial(this, copy, message);
    return copy;
  }
  /**
   * Determines whether two message of the same type have the same field values.
   * Checks for deep equality, traversing repeated fields, oneof groups, maps
   * and messages recursively.
   * Will also return true if both messages are `undefined`.
   */
  equals(a, b) {
    return reflectionEquals(this, a, b);
  }
  /**
   * Is the given value assignable to our message type
   * and contains no [excess properties](https://www.typescriptlang.org/docs/handbook/interfaces.html#excess-property-checks)?
   */
  is(arg, depth = this.defaultCheckDepth) {
    return this.refTypeCheck.is(arg, depth, false);
  }
  /**
   * Is the given value assignable to our message type,
   * regardless of [excess properties](https://www.typescriptlang.org/docs/handbook/interfaces.html#excess-property-checks)?
   */
  isAssignable(arg, depth = this.defaultCheckDepth) {
    return this.refTypeCheck.is(arg, depth, true);
  }
  /**
   * Copy partial data into the target message.
   */
  mergePartial(target, source) {
    reflectionMergePartial(this, target, source);
  }
  /**
   * Create a new message from binary format.
   */
  fromBinary(data, options) {
    let opt = binaryReadOptions(options);
    return this.internalBinaryRead(opt.readerFactory(data), data.byteLength, opt);
  }
  /**
   * Read a new message from a JSON value.
   */
  fromJson(json, options) {
    return this.internalJsonRead(json, jsonReadOptions(options));
  }
  /**
   * Read a new message from a JSON string.
   * This is equivalent to `T.fromJson(JSON.parse(json))`.
   */
  fromJsonString(json, options) {
    let value = JSON.parse(json);
    return this.fromJson(value, options);
  }
  /**
   * Write the message to canonical JSON value.
   */
  toJson(message, options) {
    return this.internalJsonWrite(message, jsonWriteOptions(options));
  }
  /**
   * Convert the message to canonical JSON string.
   * This is equivalent to `JSON.stringify(T.toJson(t))`
   */
  toJsonString(message, options) {
    var _a;
    let value = this.toJson(message, options);
    return JSON.stringify(value, null, (_a = options === null || options === void 0 ? void 0 : options.prettySpaces) !== null && _a !== void 0 ? _a : 0);
  }
  /**
   * Write the message to binary format.
   */
  toBinary(message, options) {
    let opt = binaryWriteOptions(options);
    return this.internalBinaryWrite(message, opt.writerFactory(), opt).finish();
  }
  /**
   * This is an internal method. If you just want to read a message from
   * JSON, use `fromJson()` or `fromJsonString()`.
   *
   * Reads JSON value and merges the fields into the target
   * according to protobuf rules. If the target is omitted,
   * a new instance is created first.
   */
  internalJsonRead(json, options, target) {
    if (json !== null && typeof json == "object" && !Array.isArray(json)) {
      let message = target !== null && target !== void 0 ? target : this.create();
      this.refJsonReader.read(json, message, options);
      return message;
    }
    throw new Error(`Unable to parse message ${this.typeName} from JSON ${typeofJsonValue(json)}.`);
  }
  /**
   * This is an internal method. If you just want to write a message
   * to JSON, use `toJson()` or `toJsonString().
   *
   * Writes JSON value and returns it.
   */
  internalJsonWrite(message, options) {
    return this.refJsonWriter.write(message, options);
  }
  /**
   * This is an internal method. If you just want to write a message
   * in binary format, use `toBinary()`.
   *
   * Serializes the message in binary format and appends it to the given
   * writer. Returns passed writer.
   */
  internalBinaryWrite(message, writer, options) {
    this.refBinWriter.write(message, writer, options);
    return writer;
  }
  /**
   * This is an internal method. If you just want to read a message from
   * binary data, use `fromBinary()`.
   *
   * Reads data from binary format and merges the fields into
   * the target according to protobuf rules. If the target is
   * omitted, a new instance is created first.
   */
  internalBinaryRead(reader, length, options, target) {
    let message = target !== null && target !== void 0 ? target : this.create();
    this.refBinReader.read(reader, message, options, length);
    return message;
  }
};

// ts_source/google/protobuf/timestamp_pb.ts
var Timestamp$Type = class extends MessageType {
  constructor() {
    super("google.protobuf.Timestamp", [
      {
        no: 1,
        name: "seconds",
        kind: "scalar",
        T: 3
        /*ScalarType.INT64*/
      },
      {
        no: 2,
        name: "nanos",
        kind: "scalar",
        T: 5
        /*ScalarType.INT32*/
      }
    ]);
  }
  /**
   * Creates a new `Timestamp` for the current time.
   */
  now() {
    const msg = this.create();
    const ms = Date.now();
    msg.seconds = PbLong.from(Math.floor(ms / 1e3)).toString();
    msg.nanos = ms % 1e3 * 1e6;
    return msg;
  }
  /**
   * Converts a `Timestamp` to a JavaScript Date.
   */
  toDate(message) {
    return new Date(PbLong.from(message.seconds).toNumber() * 1e3 + Math.ceil(message.nanos / 1e6));
  }
  /**
   * Converts a JavaScript Date to a `Timestamp`.
   */
  fromDate(date) {
    const msg = this.create();
    const ms = date.getTime();
    msg.seconds = PbLong.from(Math.floor(ms / 1e3)).toString();
    msg.nanos = (ms % 1e3 + (ms < 0 && ms % 1e3 !== 0 ? 1e3 : 0)) * 1e6;
    return msg;
  }
  /**
   * In JSON format, the `Timestamp` type is encoded as a string
   * in the RFC 3339 format.
   */
  internalJsonWrite(message, options) {
    let ms = PbLong.from(message.seconds).toNumber() * 1e3;
    if (ms < Date.parse("0001-01-01T00:00:00Z") || ms > Date.parse("9999-12-31T23:59:59Z"))
      throw new Error("Unable to encode Timestamp to JSON. Must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive.");
    if (message.nanos < 0)
      throw new Error("Unable to encode invalid Timestamp to JSON. Nanos must not be negative.");
    let z = "Z";
    if (message.nanos > 0) {
      let nanosStr = (message.nanos + 1e9).toString().substring(1);
      if (nanosStr.substring(3) === "000000")
        z = "." + nanosStr.substring(0, 3) + "Z";
      else if (nanosStr.substring(6) === "000")
        z = "." + nanosStr.substring(0, 6) + "Z";
      else
        z = "." + nanosStr + "Z";
    }
    return new Date(ms).toISOString().replace(".000Z", z);
  }
  /**
   * In JSON format, the `Timestamp` type is encoded as a string
   * in the RFC 3339 format.
   */
  internalJsonRead(json, options, target) {
    if (typeof json !== "string")
      throw new Error("Unable to parse Timestamp from JSON " + typeofJsonValue(json) + ".");
    let matches = json.match(/^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:Z|\.([0-9]{3,9})Z|([+-][0-9][0-9]:[0-9][0-9]))$/);
    if (!matches)
      throw new Error("Unable to parse Timestamp from JSON. Invalid format.");
    let ms = Date.parse(matches[1] + "-" + matches[2] + "-" + matches[3] + "T" + matches[4] + ":" + matches[5] + ":" + matches[6] + (matches[8] ? matches[8] : "Z"));
    if (Number.isNaN(ms))
      throw new Error("Unable to parse Timestamp from JSON. Invalid value.");
    if (ms < Date.parse("0001-01-01T00:00:00Z") || ms > Date.parse("9999-12-31T23:59:59Z"))
      throw new globalThis.Error("Unable to parse Timestamp from JSON. Must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive.");
    if (!target)
      target = this.create();
    target.seconds = PbLong.from(ms / 1e3).toString();
    target.nanos = 0;
    if (matches[7])
      target.nanos = parseInt("1" + matches[7] + "0".repeat(9 - matches[7].length)) - 1e9;
    return target;
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.seconds = "0";
    message.nanos = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* int64 seconds */
        1:
          message.seconds = reader.int64().toString();
          break;
        case /* int32 nanos */
        2:
          message.nanos = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.seconds !== "0")
      writer.tag(1, WireType.Varint).int64(message.seconds);
    if (message.nanos !== 0)
      writer.tag(2, WireType.Varint).int32(message.nanos);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Timestamp = new Timestamp$Type();

// ts_source/proto/stroppy/common_pb.ts
var Value_NullValue = /* @__PURE__ */ ((Value_NullValue2) => {
  Value_NullValue2[Value_NullValue2["NULL_VALUE"] = 0] = "NULL_VALUE";
  return Value_NullValue2;
})(Value_NullValue || {});
var Generation_Distribution_DistributionType = /* @__PURE__ */ ((Generation_Distribution_DistributionType2) => {
  Generation_Distribution_DistributionType2[Generation_Distribution_DistributionType2["NORMAL"] = 0] = "NORMAL";
  Generation_Distribution_DistributionType2[Generation_Distribution_DistributionType2["UNIFORM"] = 1] = "UNIFORM";
  Generation_Distribution_DistributionType2[Generation_Distribution_DistributionType2["ZIPF"] = 2] = "ZIPF";
  return Generation_Distribution_DistributionType2;
})(Generation_Distribution_DistributionType || {});
var OtlpExport$Type = class extends MessageType {
  constructor() {
    super("stroppy.OtlpExport", [
      {
        no: 1,
        name: "otlp_grpc_endpoint",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 3,
        name: "otlp_http_endpoint",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 4,
        name: "otlp_http_exporter_url_path",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 5,
        name: "otlp_endpoint_insecure",
        kind: "scalar",
        opt: true,
        T: 8
        /*ScalarType.BOOL*/
      },
      {
        no: 6,
        name: "otlp_headers",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "otlp_metrics_prefix",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* optional string otlp_grpc_endpoint */
        1:
          message.otlpGrpcEndpoint = reader.string();
          break;
        case /* optional string otlp_http_endpoint */
        3:
          message.otlpHttpEndpoint = reader.string();
          break;
        case /* optional string otlp_http_exporter_url_path */
        4:
          message.otlpHttpExporterUrlPath = reader.string();
          break;
        case /* optional bool otlp_endpoint_insecure */
        5:
          message.otlpEndpointInsecure = reader.bool();
          break;
        case /* optional string otlp_headers */
        6:
          message.otlpHeaders = reader.string();
          break;
        case /* optional string otlp_metrics_prefix */
        2:
          message.otlpMetricsPrefix = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.otlpGrpcEndpoint !== void 0)
      writer.tag(1, WireType.LengthDelimited).string(message.otlpGrpcEndpoint);
    if (message.otlpMetricsPrefix !== void 0)
      writer.tag(2, WireType.LengthDelimited).string(message.otlpMetricsPrefix);
    if (message.otlpHttpEndpoint !== void 0)
      writer.tag(3, WireType.LengthDelimited).string(message.otlpHttpEndpoint);
    if (message.otlpHttpExporterUrlPath !== void 0)
      writer.tag(4, WireType.LengthDelimited).string(message.otlpHttpExporterUrlPath);
    if (message.otlpEndpointInsecure !== void 0)
      writer.tag(5, WireType.Varint).bool(message.otlpEndpointInsecure);
    if (message.otlpHeaders !== void 0)
      writer.tag(6, WireType.LengthDelimited).string(message.otlpHeaders);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var OtlpExport = new OtlpExport$Type();
var Decimal$Type = class extends MessageType {
  constructor() {
    super("stroppy.Decimal", [
      {
        no: 1,
        name: "value",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.value = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string value */
        1:
          message.value = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.value !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.value);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Decimal = new Decimal$Type();
var Uuid$Type = class extends MessageType {
  constructor() {
    super("stroppy.Uuid", [
      {
        no: 1,
        name: "value",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.value = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string value */
        1:
          message.value = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.value !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.value);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Uuid = new Uuid$Type();
var DateTime$Type = class extends MessageType {
  constructor() {
    super("stroppy.DateTime", [
      { no: 1, name: "value", kind: "message", T: () => Timestamp }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* google.protobuf.Timestamp value */
        1:
          message.value = Timestamp.internalBinaryRead(reader, reader.uint32(), options, message.value);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.value)
      Timestamp.internalBinaryWrite(message.value, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var DateTime = new DateTime$Type();
var Ulid$Type = class extends MessageType {
  constructor() {
    super("stroppy.Ulid", [
      {
        no: 1,
        name: "value",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.value = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string value */
        1:
          message.value = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.value !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.value);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Ulid = new Ulid$Type();
var Value$Type = class extends MessageType {
  constructor() {
    super("stroppy.Value", [
      { no: 1, name: "null", kind: "enum", oneof: "type", T: () => ["stroppy.Value.NullValue", Value_NullValue] },
      {
        no: 2,
        name: "int32",
        kind: "scalar",
        oneof: "type",
        T: 5
        /*ScalarType.INT32*/
      },
      {
        no: 3,
        name: "uint32",
        kind: "scalar",
        oneof: "type",
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 4,
        name: "int64",
        kind: "scalar",
        oneof: "type",
        T: 3
        /*ScalarType.INT64*/
      },
      {
        no: 5,
        name: "uint64",
        kind: "scalar",
        oneof: "type",
        T: 4
        /*ScalarType.UINT64*/
      },
      {
        no: 6,
        name: "float",
        kind: "scalar",
        oneof: "type",
        T: 2
        /*ScalarType.FLOAT*/
      },
      {
        no: 7,
        name: "double",
        kind: "scalar",
        oneof: "type",
        T: 1
        /*ScalarType.DOUBLE*/
      },
      {
        no: 8,
        name: "string",
        kind: "scalar",
        oneof: "type",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 9,
        name: "bool",
        kind: "scalar",
        oneof: "type",
        T: 8
        /*ScalarType.BOOL*/
      },
      { no: 10, name: "decimal", kind: "message", oneof: "type", T: () => Decimal },
      { no: 11, name: "uuid", kind: "message", oneof: "type", T: () => Uuid },
      { no: 12, name: "datetime", kind: "message", oneof: "type", T: () => DateTime },
      { no: 13, name: "struct", kind: "message", oneof: "type", T: () => Value_Struct },
      { no: 14, name: "list", kind: "message", oneof: "type", T: () => Value_List },
      {
        no: 101,
        name: "key",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.type = { oneofKind: void 0 };
    message.key = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.Value.NullValue null */
        1:
          message.type = {
            oneofKind: "null",
            null: reader.int32()
          };
          break;
        case /* int32 int32 */
        2:
          message.type = {
            oneofKind: "int32",
            int32: reader.int32()
          };
          break;
        case /* uint32 uint32 */
        3:
          message.type = {
            oneofKind: "uint32",
            uint32: reader.uint32()
          };
          break;
        case /* int64 int64 */
        4:
          message.type = {
            oneofKind: "int64",
            int64: reader.int64().toString()
          };
          break;
        case /* uint64 uint64 */
        5:
          message.type = {
            oneofKind: "uint64",
            uint64: reader.uint64().toString()
          };
          break;
        case /* float float */
        6:
          message.type = {
            oneofKind: "float",
            float: reader.float()
          };
          break;
        case /* double double */
        7:
          message.type = {
            oneofKind: "double",
            double: reader.double()
          };
          break;
        case /* string string */
        8:
          message.type = {
            oneofKind: "string",
            string: reader.string()
          };
          break;
        case /* bool bool */
        9:
          message.type = {
            oneofKind: "bool",
            bool: reader.bool()
          };
          break;
        case /* stroppy.Decimal decimal */
        10:
          message.type = {
            oneofKind: "decimal",
            decimal: Decimal.internalBinaryRead(reader, reader.uint32(), options, message.type.decimal)
          };
          break;
        case /* stroppy.Uuid uuid */
        11:
          message.type = {
            oneofKind: "uuid",
            uuid: Uuid.internalBinaryRead(reader, reader.uint32(), options, message.type.uuid)
          };
          break;
        case /* stroppy.DateTime datetime */
        12:
          message.type = {
            oneofKind: "datetime",
            datetime: DateTime.internalBinaryRead(reader, reader.uint32(), options, message.type.datetime)
          };
          break;
        case /* stroppy.Value.Struct struct */
        13:
          message.type = {
            oneofKind: "struct",
            struct: Value_Struct.internalBinaryRead(reader, reader.uint32(), options, message.type.struct)
          };
          break;
        case /* stroppy.Value.List list */
        14:
          message.type = {
            oneofKind: "list",
            list: Value_List.internalBinaryRead(reader, reader.uint32(), options, message.type.list)
          };
          break;
        case /* string key */
        101:
          message.key = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.type.oneofKind === "null")
      writer.tag(1, WireType.Varint).int32(message.type.null);
    if (message.type.oneofKind === "int32")
      writer.tag(2, WireType.Varint).int32(message.type.int32);
    if (message.type.oneofKind === "uint32")
      writer.tag(3, WireType.Varint).uint32(message.type.uint32);
    if (message.type.oneofKind === "int64")
      writer.tag(4, WireType.Varint).int64(message.type.int64);
    if (message.type.oneofKind === "uint64")
      writer.tag(5, WireType.Varint).uint64(message.type.uint64);
    if (message.type.oneofKind === "float")
      writer.tag(6, WireType.Bit32).float(message.type.float);
    if (message.type.oneofKind === "double")
      writer.tag(7, WireType.Bit64).double(message.type.double);
    if (message.type.oneofKind === "string")
      writer.tag(8, WireType.LengthDelimited).string(message.type.string);
    if (message.type.oneofKind === "bool")
      writer.tag(9, WireType.Varint).bool(message.type.bool);
    if (message.type.oneofKind === "decimal")
      Decimal.internalBinaryWrite(message.type.decimal, writer.tag(10, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "uuid")
      Uuid.internalBinaryWrite(message.type.uuid, writer.tag(11, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "datetime")
      DateTime.internalBinaryWrite(message.type.datetime, writer.tag(12, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "struct")
      Value_Struct.internalBinaryWrite(message.type.struct, writer.tag(13, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "list")
      Value_List.internalBinaryWrite(message.type.list, writer.tag(14, WireType.LengthDelimited).fork(), options).join();
    if (message.key !== "")
      writer.tag(101, WireType.LengthDelimited).string(message.key);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Value = new Value$Type();
var Value_List$Type = class extends MessageType {
  constructor() {
    super("stroppy.Value.List", [
      { no: 1, name: "values", kind: "message", repeat: 2, T: () => Value }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.values = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* repeated stroppy.Value values */
        1:
          message.values.push(Value.internalBinaryRead(reader, reader.uint32(), options));
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    for (let i = 0; i < message.values.length; i++)
      Value.internalBinaryWrite(message.values[i], writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Value_List = new Value_List$Type();
var Value_Struct$Type = class extends MessageType {
  constructor() {
    super("stroppy.Value.Struct", [
      { no: 1, name: "fields", kind: "message", repeat: 2, T: () => Value }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.fields = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* repeated stroppy.Value fields */
        1:
          message.fields.push(Value.internalBinaryRead(reader, reader.uint32(), options));
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    for (let i = 0; i < message.fields.length; i++)
      Value.internalBinaryWrite(message.fields[i], writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Value_Struct = new Value_Struct$Type();
var Generation$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation", []);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation = new Generation$Type();
var Generation_Alphabet$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Alphabet", [
      { no: 1, name: "ranges", kind: "message", repeat: 2, T: () => Generation_Range_UInt32 }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.ranges = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* repeated stroppy.Generation.Range.UInt32 ranges */
        1:
          message.ranges.push(Generation_Range_UInt32.internalBinaryRead(reader, reader.uint32(), options));
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    for (let i = 0; i < message.ranges.length; i++)
      Generation_Range_UInt32.internalBinaryWrite(message.ranges[i], writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Alphabet = new Generation_Alphabet$Type();
var Generation_Distribution$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Distribution", [
      { no: 1, name: "type", kind: "enum", T: () => ["stroppy.Generation.Distribution.DistributionType", Generation_Distribution_DistributionType] },
      {
        no: 2,
        name: "screw",
        kind: "scalar",
        T: 1
        /*ScalarType.DOUBLE*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.type = 0;
    message.screw = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.Generation.Distribution.DistributionType type */
        1:
          message.type = reader.int32();
          break;
        case /* double screw */
        2:
          message.screw = reader.double();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.type !== 0)
      writer.tag(1, WireType.Varint).int32(message.type);
    if (message.screw !== 0)
      writer.tag(2, WireType.Bit64).double(message.screw);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Distribution = new Generation_Distribution$Type();
var Generation_Range$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range", []);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range = new Generation_Range$Type();
var Generation_Range_Bool$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.Bool", [
      {
        no: 1,
        name: "ratio",
        kind: "scalar",
        T: 2
        /*ScalarType.FLOAT*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.ratio = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* float ratio */
        1:
          message.ratio = reader.float();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.ratio !== 0)
      writer.tag(1, WireType.Bit32).float(message.ratio);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_Bool = new Generation_Range_Bool$Type();
var Generation_Range_String$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.String", [
      { no: 1, name: "alphabet", kind: "message", T: () => Generation_Alphabet },
      {
        no: 2,
        name: "min_len",
        kind: "scalar",
        opt: true,
        T: 4
        /*ScalarType.UINT64*/
      },
      {
        no: 3,
        name: "max_len",
        kind: "scalar",
        T: 4
        /*ScalarType.UINT64*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.maxLen = "0";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* optional stroppy.Generation.Alphabet alphabet */
        1:
          message.alphabet = Generation_Alphabet.internalBinaryRead(reader, reader.uint32(), options, message.alphabet);
          break;
        case /* optional uint64 min_len */
        2:
          message.minLen = reader.uint64().toString();
          break;
        case /* uint64 max_len */
        3:
          message.maxLen = reader.uint64().toString();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.alphabet)
      Generation_Alphabet.internalBinaryWrite(message.alphabet, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.minLen !== void 0)
      writer.tag(2, WireType.Varint).uint64(message.minLen);
    if (message.maxLen !== "0")
      writer.tag(3, WireType.Varint).uint64(message.maxLen);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_String = new Generation_Range_String$Type();
var Generation_Range_AnyString$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.AnyString", [
      {
        no: 1,
        name: "min",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "max",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.min = "";
    message.max = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string min */
        1:
          message.min = reader.string();
          break;
        case /* string max */
        2:
          message.max = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.min);
    if (message.max !== "")
      writer.tag(2, WireType.LengthDelimited).string(message.max);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_AnyString = new Generation_Range_AnyString$Type();
var Generation_Range_Float$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.Float", [
      {
        no: 1,
        name: "min",
        kind: "scalar",
        opt: true,
        T: 2
        /*ScalarType.FLOAT*/
      },
      {
        no: 2,
        name: "max",
        kind: "scalar",
        T: 2
        /*ScalarType.FLOAT*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.max = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* optional float min */
        1:
          message.min = reader.float();
          break;
        case /* float max */
        2:
          message.max = reader.float();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min !== void 0)
      writer.tag(1, WireType.Bit32).float(message.min);
    if (message.max !== 0)
      writer.tag(2, WireType.Bit32).float(message.max);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_Float = new Generation_Range_Float$Type();
var Generation_Range_Double$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.Double", [
      {
        no: 1,
        name: "min",
        kind: "scalar",
        opt: true,
        T: 1
        /*ScalarType.DOUBLE*/
      },
      {
        no: 2,
        name: "max",
        kind: "scalar",
        T: 1
        /*ScalarType.DOUBLE*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.max = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* optional double min */
        1:
          message.min = reader.double();
          break;
        case /* double max */
        2:
          message.max = reader.double();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min !== void 0)
      writer.tag(1, WireType.Bit64).double(message.min);
    if (message.max !== 0)
      writer.tag(2, WireType.Bit64).double(message.max);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_Double = new Generation_Range_Double$Type();
var Generation_Range_Int32$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.Int32", [
      {
        no: 1,
        name: "min",
        kind: "scalar",
        opt: true,
        T: 5
        /*ScalarType.INT32*/
      },
      {
        no: 2,
        name: "max",
        kind: "scalar",
        T: 5
        /*ScalarType.INT32*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.max = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* optional int32 min */
        1:
          message.min = reader.int32();
          break;
        case /* int32 max */
        2:
          message.max = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min !== void 0)
      writer.tag(1, WireType.Varint).int32(message.min);
    if (message.max !== 0)
      writer.tag(2, WireType.Varint).int32(message.max);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_Int32 = new Generation_Range_Int32$Type();
var Generation_Range_Int64$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.Int64", [
      {
        no: 1,
        name: "min",
        kind: "scalar",
        opt: true,
        T: 3
        /*ScalarType.INT64*/
      },
      {
        no: 2,
        name: "max",
        kind: "scalar",
        T: 3
        /*ScalarType.INT64*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.max = "0";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* optional int64 min */
        1:
          message.min = reader.int64().toString();
          break;
        case /* int64 max */
        2:
          message.max = reader.int64().toString();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min !== void 0)
      writer.tag(1, WireType.Varint).int64(message.min);
    if (message.max !== "0")
      writer.tag(2, WireType.Varint).int64(message.max);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_Int64 = new Generation_Range_Int64$Type();
var Generation_Range_UInt32$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.UInt32", [
      {
        no: 1,
        name: "min",
        kind: "scalar",
        opt: true,
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 2,
        name: "max",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.max = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* optional uint32 min */
        1:
          message.min = reader.uint32();
          break;
        case /* uint32 max */
        2:
          message.max = reader.uint32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min !== void 0)
      writer.tag(1, WireType.Varint).uint32(message.min);
    if (message.max !== 0)
      writer.tag(2, WireType.Varint).uint32(message.max);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_UInt32 = new Generation_Range_UInt32$Type();
var Generation_Range_UInt64$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.UInt64", [
      {
        no: 1,
        name: "min",
        kind: "scalar",
        opt: true,
        T: 4
        /*ScalarType.UINT64*/
      },
      {
        no: 2,
        name: "max",
        kind: "scalar",
        T: 4
        /*ScalarType.UINT64*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.max = "0";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* optional uint64 min */
        1:
          message.min = reader.uint64().toString();
          break;
        case /* uint64 max */
        2:
          message.max = reader.uint64().toString();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min !== void 0)
      writer.tag(1, WireType.Varint).uint64(message.min);
    if (message.max !== "0")
      writer.tag(2, WireType.Varint).uint64(message.max);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_UInt64 = new Generation_Range_UInt64$Type();
var Generation_Range_DecimalRange$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.DecimalRange", [
      { no: 2, name: "float", kind: "message", oneof: "type", T: () => Generation_Range_Float },
      { no: 3, name: "double", kind: "message", oneof: "type", T: () => Generation_Range_Double },
      { no: 4, name: "string", kind: "message", oneof: "type", T: () => Generation_Range_AnyString }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.type = { oneofKind: void 0 };
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.Generation.Range.Float float */
        2:
          message.type = {
            oneofKind: "float",
            float: Generation_Range_Float.internalBinaryRead(reader, reader.uint32(), options, message.type.float)
          };
          break;
        case /* stroppy.Generation.Range.Double double */
        3:
          message.type = {
            oneofKind: "double",
            double: Generation_Range_Double.internalBinaryRead(reader, reader.uint32(), options, message.type.double)
          };
          break;
        case /* stroppy.Generation.Range.AnyString string */
        4:
          message.type = {
            oneofKind: "string",
            string: Generation_Range_AnyString.internalBinaryRead(reader, reader.uint32(), options, message.type.string)
          };
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.type.oneofKind === "float")
      Generation_Range_Float.internalBinaryWrite(message.type.float, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "double")
      Generation_Range_Double.internalBinaryWrite(message.type.double, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "string")
      Generation_Range_AnyString.internalBinaryWrite(message.type.string, writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_DecimalRange = new Generation_Range_DecimalRange$Type();
var Generation_Range_DateTime$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.DateTime", [
      { no: 2, name: "string", kind: "message", oneof: "type", T: () => Generation_Range_AnyString },
      { no: 3, name: "timestamp_pb", kind: "message", oneof: "type", T: () => Generation_Range_DateTime_TimestampPb },
      { no: 4, name: "timestamp", kind: "message", oneof: "type", T: () => Generation_Range_DateTime_TimestampUnix }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.type = { oneofKind: void 0 };
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.Generation.Range.AnyString string */
        2:
          message.type = {
            oneofKind: "string",
            string: Generation_Range_AnyString.internalBinaryRead(reader, reader.uint32(), options, message.type.string)
          };
          break;
        case /* stroppy.Generation.Range.DateTime.TimestampPb timestamp_pb */
        3:
          message.type = {
            oneofKind: "timestampPb",
            timestampPb: Generation_Range_DateTime_TimestampPb.internalBinaryRead(reader, reader.uint32(), options, message.type.timestampPb)
          };
          break;
        case /* stroppy.Generation.Range.DateTime.TimestampUnix timestamp */
        4:
          message.type = {
            oneofKind: "timestamp",
            timestamp: Generation_Range_DateTime_TimestampUnix.internalBinaryRead(reader, reader.uint32(), options, message.type.timestamp)
          };
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.type.oneofKind === "string")
      Generation_Range_AnyString.internalBinaryWrite(message.type.string, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "timestampPb")
      Generation_Range_DateTime_TimestampPb.internalBinaryWrite(message.type.timestampPb, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "timestamp")
      Generation_Range_DateTime_TimestampUnix.internalBinaryWrite(message.type.timestamp, writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_DateTime = new Generation_Range_DateTime$Type();
var Generation_Range_DateTime_TimestampPb$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.DateTime.TimestampPb", [
      { no: 1, name: "min", kind: "message", T: () => Timestamp },
      { no: 2, name: "max", kind: "message", T: () => Timestamp }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* google.protobuf.Timestamp min */
        1:
          message.min = Timestamp.internalBinaryRead(reader, reader.uint32(), options, message.min);
          break;
        case /* google.protobuf.Timestamp max */
        2:
          message.max = Timestamp.internalBinaryRead(reader, reader.uint32(), options, message.max);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min)
      Timestamp.internalBinaryWrite(message.min, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.max)
      Timestamp.internalBinaryWrite(message.max, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_DateTime_TimestampPb = new Generation_Range_DateTime_TimestampPb$Type();
var Generation_Range_DateTime_TimestampUnix$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Range.DateTime.TimestampUnix", [
      {
        no: 1,
        name: "min",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 2,
        name: "max",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.min = 0;
    message.max = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* uint32 min */
        1:
          message.min = reader.uint32();
          break;
        case /* uint32 max */
        2:
          message.max = reader.uint32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.min !== 0)
      writer.tag(1, WireType.Varint).uint32(message.min);
    if (message.max !== 0)
      writer.tag(2, WireType.Varint).uint32(message.max);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Range_DateTime_TimestampUnix = new Generation_Range_DateTime_TimestampUnix$Type();
var Generation_Rule$Type = class extends MessageType {
  constructor() {
    super("stroppy.Generation.Rule", [
      { no: 1, name: "int32_range", kind: "message", oneof: "kind", T: () => Generation_Range_Int32 },
      { no: 2, name: "int64_range", kind: "message", oneof: "kind", T: () => Generation_Range_Int64 },
      { no: 3, name: "uint32_range", kind: "message", oneof: "kind", T: () => Generation_Range_UInt32 },
      { no: 4, name: "uint64_range", kind: "message", oneof: "kind", T: () => Generation_Range_UInt64 },
      { no: 5, name: "float_range", kind: "message", oneof: "kind", T: () => Generation_Range_Float },
      { no: 6, name: "double_range", kind: "message", oneof: "kind", T: () => Generation_Range_Double },
      { no: 7, name: "decimal_range", kind: "message", oneof: "kind", T: () => Generation_Range_DecimalRange },
      { no: 8, name: "string_range", kind: "message", oneof: "kind", T: () => Generation_Range_String },
      { no: 9, name: "bool_range", kind: "message", oneof: "kind", T: () => Generation_Range_Bool },
      { no: 10, name: "datetime_range", kind: "message", oneof: "kind", T: () => Generation_Range_DateTime },
      {
        no: 11,
        name: "int32_const",
        kind: "scalar",
        oneof: "kind",
        T: 5
        /*ScalarType.INT32*/
      },
      {
        no: 12,
        name: "int64_const",
        kind: "scalar",
        oneof: "kind",
        T: 3
        /*ScalarType.INT64*/
      },
      {
        no: 13,
        name: "uint32_const",
        kind: "scalar",
        oneof: "kind",
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 14,
        name: "uint64_const",
        kind: "scalar",
        oneof: "kind",
        T: 4
        /*ScalarType.UINT64*/
      },
      {
        no: 15,
        name: "float_const",
        kind: "scalar",
        oneof: "kind",
        T: 2
        /*ScalarType.FLOAT*/
      },
      {
        no: 16,
        name: "double_const",
        kind: "scalar",
        oneof: "kind",
        T: 1
        /*ScalarType.DOUBLE*/
      },
      { no: 17, name: "decimal_const", kind: "message", oneof: "kind", T: () => Decimal },
      {
        no: 18,
        name: "string_const",
        kind: "scalar",
        oneof: "kind",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 19,
        name: "bool_const",
        kind: "scalar",
        oneof: "kind",
        T: 8
        /*ScalarType.BOOL*/
      },
      { no: 20, name: "datetime_const", kind: "message", oneof: "kind", T: () => DateTime },
      { no: 30, name: "distribution", kind: "message", T: () => Generation_Distribution },
      {
        no: 31,
        name: "null_percentage",
        kind: "scalar",
        opt: true,
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 32,
        name: "unique",
        kind: "scalar",
        opt: true,
        T: 8
        /*ScalarType.BOOL*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.kind = { oneofKind: void 0 };
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.Generation.Range.Int32 int32_range */
        1:
          message.kind = {
            oneofKind: "int32Range",
            int32Range: Generation_Range_Int32.internalBinaryRead(reader, reader.uint32(), options, message.kind.int32Range)
          };
          break;
        case /* stroppy.Generation.Range.Int64 int64_range */
        2:
          message.kind = {
            oneofKind: "int64Range",
            int64Range: Generation_Range_Int64.internalBinaryRead(reader, reader.uint32(), options, message.kind.int64Range)
          };
          break;
        case /* stroppy.Generation.Range.UInt32 uint32_range */
        3:
          message.kind = {
            oneofKind: "uint32Range",
            uint32Range: Generation_Range_UInt32.internalBinaryRead(reader, reader.uint32(), options, message.kind.uint32Range)
          };
          break;
        case /* stroppy.Generation.Range.UInt64 uint64_range */
        4:
          message.kind = {
            oneofKind: "uint64Range",
            uint64Range: Generation_Range_UInt64.internalBinaryRead(reader, reader.uint32(), options, message.kind.uint64Range)
          };
          break;
        case /* stroppy.Generation.Range.Float float_range */
        5:
          message.kind = {
            oneofKind: "floatRange",
            floatRange: Generation_Range_Float.internalBinaryRead(reader, reader.uint32(), options, message.kind.floatRange)
          };
          break;
        case /* stroppy.Generation.Range.Double double_range */
        6:
          message.kind = {
            oneofKind: "doubleRange",
            doubleRange: Generation_Range_Double.internalBinaryRead(reader, reader.uint32(), options, message.kind.doubleRange)
          };
          break;
        case /* stroppy.Generation.Range.DecimalRange decimal_range */
        7:
          message.kind = {
            oneofKind: "decimalRange",
            decimalRange: Generation_Range_DecimalRange.internalBinaryRead(reader, reader.uint32(), options, message.kind.decimalRange)
          };
          break;
        case /* stroppy.Generation.Range.String string_range */
        8:
          message.kind = {
            oneofKind: "stringRange",
            stringRange: Generation_Range_String.internalBinaryRead(reader, reader.uint32(), options, message.kind.stringRange)
          };
          break;
        case /* stroppy.Generation.Range.Bool bool_range */
        9:
          message.kind = {
            oneofKind: "boolRange",
            boolRange: Generation_Range_Bool.internalBinaryRead(reader, reader.uint32(), options, message.kind.boolRange)
          };
          break;
        case /* stroppy.Generation.Range.DateTime datetime_range */
        10:
          message.kind = {
            oneofKind: "datetimeRange",
            datetimeRange: Generation_Range_DateTime.internalBinaryRead(reader, reader.uint32(), options, message.kind.datetimeRange)
          };
          break;
        case /* int32 int32_const */
        11:
          message.kind = {
            oneofKind: "int32Const",
            int32Const: reader.int32()
          };
          break;
        case /* int64 int64_const */
        12:
          message.kind = {
            oneofKind: "int64Const",
            int64Const: reader.int64().toString()
          };
          break;
        case /* uint32 uint32_const */
        13:
          message.kind = {
            oneofKind: "uint32Const",
            uint32Const: reader.uint32()
          };
          break;
        case /* uint64 uint64_const */
        14:
          message.kind = {
            oneofKind: "uint64Const",
            uint64Const: reader.uint64().toString()
          };
          break;
        case /* float float_const */
        15:
          message.kind = {
            oneofKind: "floatConst",
            floatConst: reader.float()
          };
          break;
        case /* double double_const */
        16:
          message.kind = {
            oneofKind: "doubleConst",
            doubleConst: reader.double()
          };
          break;
        case /* stroppy.Decimal decimal_const */
        17:
          message.kind = {
            oneofKind: "decimalConst",
            decimalConst: Decimal.internalBinaryRead(reader, reader.uint32(), options, message.kind.decimalConst)
          };
          break;
        case /* string string_const */
        18:
          message.kind = {
            oneofKind: "stringConst",
            stringConst: reader.string()
          };
          break;
        case /* bool bool_const */
        19:
          message.kind = {
            oneofKind: "boolConst",
            boolConst: reader.bool()
          };
          break;
        case /* stroppy.DateTime datetime_const */
        20:
          message.kind = {
            oneofKind: "datetimeConst",
            datetimeConst: DateTime.internalBinaryRead(reader, reader.uint32(), options, message.kind.datetimeConst)
          };
          break;
        case /* optional stroppy.Generation.Distribution distribution */
        30:
          message.distribution = Generation_Distribution.internalBinaryRead(reader, reader.uint32(), options, message.distribution);
          break;
        case /* optional uint32 null_percentage */
        31:
          message.nullPercentage = reader.uint32();
          break;
        case /* optional bool unique */
        32:
          message.unique = reader.bool();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.kind.oneofKind === "int32Range")
      Generation_Range_Int32.internalBinaryWrite(message.kind.int32Range, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "int64Range")
      Generation_Range_Int64.internalBinaryWrite(message.kind.int64Range, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "uint32Range")
      Generation_Range_UInt32.internalBinaryWrite(message.kind.uint32Range, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "uint64Range")
      Generation_Range_UInt64.internalBinaryWrite(message.kind.uint64Range, writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "floatRange")
      Generation_Range_Float.internalBinaryWrite(message.kind.floatRange, writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "doubleRange")
      Generation_Range_Double.internalBinaryWrite(message.kind.doubleRange, writer.tag(6, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "decimalRange")
      Generation_Range_DecimalRange.internalBinaryWrite(message.kind.decimalRange, writer.tag(7, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "stringRange")
      Generation_Range_String.internalBinaryWrite(message.kind.stringRange, writer.tag(8, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "boolRange")
      Generation_Range_Bool.internalBinaryWrite(message.kind.boolRange, writer.tag(9, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "datetimeRange")
      Generation_Range_DateTime.internalBinaryWrite(message.kind.datetimeRange, writer.tag(10, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "int32Const")
      writer.tag(11, WireType.Varint).int32(message.kind.int32Const);
    if (message.kind.oneofKind === "int64Const")
      writer.tag(12, WireType.Varint).int64(message.kind.int64Const);
    if (message.kind.oneofKind === "uint32Const")
      writer.tag(13, WireType.Varint).uint32(message.kind.uint32Const);
    if (message.kind.oneofKind === "uint64Const")
      writer.tag(14, WireType.Varint).uint64(message.kind.uint64Const);
    if (message.kind.oneofKind === "floatConst")
      writer.tag(15, WireType.Bit32).float(message.kind.floatConst);
    if (message.kind.oneofKind === "doubleConst")
      writer.tag(16, WireType.Bit64).double(message.kind.doubleConst);
    if (message.kind.oneofKind === "decimalConst")
      Decimal.internalBinaryWrite(message.kind.decimalConst, writer.tag(17, WireType.LengthDelimited).fork(), options).join();
    if (message.kind.oneofKind === "stringConst")
      writer.tag(18, WireType.LengthDelimited).string(message.kind.stringConst);
    if (message.kind.oneofKind === "boolConst")
      writer.tag(19, WireType.Varint).bool(message.kind.boolConst);
    if (message.kind.oneofKind === "datetimeConst")
      DateTime.internalBinaryWrite(message.kind.datetimeConst, writer.tag(20, WireType.LengthDelimited).fork(), options).join();
    if (message.distribution)
      Generation_Distribution.internalBinaryWrite(message.distribution, writer.tag(30, WireType.LengthDelimited).fork(), options).join();
    if (message.nullPercentage !== void 0)
      writer.tag(31, WireType.Varint).uint32(message.nullPercentage);
    if (message.unique !== void 0)
      writer.tag(32, WireType.Varint).bool(message.unique);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Generation_Rule = new Generation_Rule$Type();

// ts_source/proto/stroppy/descriptor_pb.ts
var InsertMethod = /* @__PURE__ */ ((InsertMethod2) => {
  InsertMethod2[InsertMethod2["PLAIN_QUERY"] = 0] = "PLAIN_QUERY";
  InsertMethod2[InsertMethod2["COPY_FROM"] = 1] = "COPY_FROM";
  return InsertMethod2;
})(InsertMethod || {});
var TxIsolationLevel = /* @__PURE__ */ ((TxIsolationLevel2) => {
  TxIsolationLevel2[TxIsolationLevel2["UNSPECIFIED"] = 0] = "UNSPECIFIED";
  TxIsolationLevel2[TxIsolationLevel2["READ_UNCOMMITTED"] = 1] = "READ_UNCOMMITTED";
  TxIsolationLevel2[TxIsolationLevel2["READ_COMMITTED"] = 2] = "READ_COMMITTED";
  TxIsolationLevel2[TxIsolationLevel2["REPEATABLE_READ"] = 3] = "REPEATABLE_READ";
  TxIsolationLevel2[TxIsolationLevel2["SERIALIZABLE"] = 4] = "SERIALIZABLE";
  return TxIsolationLevel2;
})(TxIsolationLevel || {});
var IndexDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.IndexDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "columns",
        kind: "scalar",
        repeat: 2,
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 3,
        name: "type",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 4,
        name: "unique",
        kind: "scalar",
        T: 8
        /*ScalarType.BOOL*/
      },
      { no: 5, name: "db_specific", kind: "message", T: () => Value_Struct }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.columns = [];
    message.type = "";
    message.unique = false;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* repeated string columns */
        2:
          message.columns.push(reader.string());
          break;
        case /* string type */
        3:
          message.type = reader.string();
          break;
        case /* bool unique */
        4:
          message.unique = reader.bool();
          break;
        case /* optional stroppy.Value.Struct db_specific */
        5:
          message.dbSpecific = Value_Struct.internalBinaryRead(reader, reader.uint32(), options, message.dbSpecific);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    for (let i = 0; i < message.columns.length; i++)
      writer.tag(2, WireType.LengthDelimited).string(message.columns[i]);
    if (message.type !== "")
      writer.tag(3, WireType.LengthDelimited).string(message.type);
    if (message.unique !== false)
      writer.tag(4, WireType.Varint).bool(message.unique);
    if (message.dbSpecific)
      Value_Struct.internalBinaryWrite(message.dbSpecific, writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var IndexDescriptor = new IndexDescriptor$Type();
var ColumnDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.ColumnDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "sql_type",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 3,
        name: "nullable",
        kind: "scalar",
        opt: true,
        T: 8
        /*ScalarType.BOOL*/
      },
      {
        no: 4,
        name: "primary_key",
        kind: "scalar",
        opt: true,
        T: 8
        /*ScalarType.BOOL*/
      },
      {
        no: 5,
        name: "unique",
        kind: "scalar",
        opt: true,
        T: 8
        /*ScalarType.BOOL*/
      },
      {
        no: 6,
        name: "constraint",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.sqlType = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* string sql_type */
        2:
          message.sqlType = reader.string();
          break;
        case /* optional bool nullable */
        3:
          message.nullable = reader.bool();
          break;
        case /* optional bool primary_key */
        4:
          message.primaryKey = reader.bool();
          break;
        case /* optional bool unique */
        5:
          message.unique = reader.bool();
          break;
        case /* optional string constraint */
        6:
          message.constraint = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.sqlType !== "")
      writer.tag(2, WireType.LengthDelimited).string(message.sqlType);
    if (message.nullable !== void 0)
      writer.tag(3, WireType.Varint).bool(message.nullable);
    if (message.primaryKey !== void 0)
      writer.tag(4, WireType.Varint).bool(message.primaryKey);
    if (message.unique !== void 0)
      writer.tag(5, WireType.Varint).bool(message.unique);
    if (message.constraint !== void 0)
      writer.tag(6, WireType.LengthDelimited).string(message.constraint);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var ColumnDescriptor = new ColumnDescriptor$Type();
var TableDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.TableDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 3, name: "table_indexes", kind: "message", repeat: 2, T: () => IndexDescriptor },
      {
        no: 5,
        name: "constraint",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 6, name: "db_specific", kind: "message", T: () => Value_Struct },
      { no: 100, name: "columns", kind: "message", repeat: 2, T: () => ColumnDescriptor }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.tableIndexes = [];
    message.columns = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* repeated stroppy.IndexDescriptor table_indexes */
        3:
          message.tableIndexes.push(IndexDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* optional string constraint */
        5:
          message.constraint = reader.string();
          break;
        case /* optional stroppy.Value.Struct db_specific */
        6:
          message.dbSpecific = Value_Struct.internalBinaryRead(reader, reader.uint32(), options, message.dbSpecific);
          break;
        case /* repeated stroppy.ColumnDescriptor columns */
        100:
          message.columns.push(ColumnDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    for (let i = 0; i < message.tableIndexes.length; i++)
      IndexDescriptor.internalBinaryWrite(message.tableIndexes[i], writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.constraint !== void 0)
      writer.tag(5, WireType.LengthDelimited).string(message.constraint);
    if (message.dbSpecific)
      Value_Struct.internalBinaryWrite(message.dbSpecific, writer.tag(6, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.columns.length; i++)
      ColumnDescriptor.internalBinaryWrite(message.columns[i], writer.tag(100, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var TableDescriptor = new TableDescriptor$Type();
var InsertDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.InsertDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "table_name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 3, name: "method", kind: "enum", opt: true, T: () => ["stroppy.InsertMethod", InsertMethod] },
      { no: 4, name: "params", kind: "message", repeat: 2, T: () => QueryParamDescriptor },
      { no: 5, name: "groups", kind: "message", repeat: 2, T: () => QueryParamGroup }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.tableName = "";
    message.params = [];
    message.groups = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* string table_name */
        2:
          message.tableName = reader.string();
          break;
        case /* optional stroppy.InsertMethod method */
        3:
          message.method = reader.int32();
          break;
        case /* repeated stroppy.QueryParamDescriptor params */
        4:
          message.params.push(QueryParamDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* repeated stroppy.QueryParamGroup groups */
        5:
          message.groups.push(QueryParamGroup.internalBinaryRead(reader, reader.uint32(), options));
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.tableName !== "")
      writer.tag(2, WireType.LengthDelimited).string(message.tableName);
    if (message.method !== void 0)
      writer.tag(3, WireType.Varint).int32(message.method);
    for (let i = 0; i < message.params.length; i++)
      QueryParamDescriptor.internalBinaryWrite(message.params[i], writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.groups.length; i++)
      QueryParamGroup.internalBinaryWrite(message.groups[i], writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var InsertDescriptor = new InsertDescriptor$Type();
var QueryParamDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.QueryParamDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "replace_regex",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 3, name: "generation_rule", kind: "message", T: () => Generation_Rule },
      { no: 4, name: "db_specific", kind: "message", T: () => Value_Struct }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* optional string replace_regex */
        2:
          message.replaceRegex = reader.string();
          break;
        case /* stroppy.Generation.Rule generation_rule */
        3:
          message.generationRule = Generation_Rule.internalBinaryRead(reader, reader.uint32(), options, message.generationRule);
          break;
        case /* optional stroppy.Value.Struct db_specific */
        4:
          message.dbSpecific = Value_Struct.internalBinaryRead(reader, reader.uint32(), options, message.dbSpecific);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.replaceRegex !== void 0)
      writer.tag(2, WireType.LengthDelimited).string(message.replaceRegex);
    if (message.generationRule)
      Generation_Rule.internalBinaryWrite(message.generationRule, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.dbSpecific)
      Value_Struct.internalBinaryWrite(message.dbSpecific, writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var QueryParamDescriptor = new QueryParamDescriptor$Type();
var QueryParamGroup$Type = class extends MessageType {
  constructor() {
    super("stroppy.QueryParamGroup", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 2, name: "params", kind: "message", repeat: 2, T: () => QueryParamDescriptor }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.params = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* repeated stroppy.QueryParamDescriptor params */
        2:
          message.params.push(QueryParamDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    for (let i = 0; i < message.params.length; i++)
      QueryParamDescriptor.internalBinaryWrite(message.params[i], writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var QueryParamGroup = new QueryParamGroup$Type();
var QueryDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.QueryDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "sql",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 3, name: "params", kind: "message", repeat: 2, T: () => QueryParamDescriptor },
      { no: 4, name: "groups", kind: "message", repeat: 2, T: () => QueryParamGroup },
      { no: 5, name: "db_specific", kind: "message", T: () => Value_Struct }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.sql = "";
    message.params = [];
    message.groups = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* string sql */
        2:
          message.sql = reader.string();
          break;
        case /* repeated stroppy.QueryParamDescriptor params */
        3:
          message.params.push(QueryParamDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* repeated stroppy.QueryParamGroup groups */
        4:
          message.groups.push(QueryParamGroup.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* optional stroppy.Value.Struct db_specific */
        5:
          message.dbSpecific = Value_Struct.internalBinaryRead(reader, reader.uint32(), options, message.dbSpecific);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.sql !== "")
      writer.tag(2, WireType.LengthDelimited).string(message.sql);
    for (let i = 0; i < message.params.length; i++)
      QueryParamDescriptor.internalBinaryWrite(message.params[i], writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.groups.length; i++)
      QueryParamGroup.internalBinaryWrite(message.groups[i], writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    if (message.dbSpecific)
      Value_Struct.internalBinaryWrite(message.dbSpecific, writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var QueryDescriptor = new QueryDescriptor$Type();
var TransactionDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.TransactionDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 2, name: "isolation_level", kind: "enum", T: () => ["stroppy.TxIsolationLevel", TxIsolationLevel] },
      { no: 3, name: "queries", kind: "message", repeat: 2, T: () => QueryDescriptor },
      { no: 4, name: "params", kind: "message", repeat: 2, T: () => QueryParamDescriptor },
      { no: 5, name: "groups", kind: "message", repeat: 2, T: () => QueryParamGroup },
      { no: 6, name: "db_specific", kind: "message", T: () => Value_Struct }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.isolationLevel = 0;
    message.queries = [];
    message.params = [];
    message.groups = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* stroppy.TxIsolationLevel isolation_level */
        2:
          message.isolationLevel = reader.int32();
          break;
        case /* repeated stroppy.QueryDescriptor queries */
        3:
          message.queries.push(QueryDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* repeated stroppy.QueryParamDescriptor params */
        4:
          message.params.push(QueryParamDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* repeated stroppy.QueryParamGroup groups */
        5:
          message.groups.push(QueryParamGroup.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* optional stroppy.Value.Struct db_specific */
        6:
          message.dbSpecific = Value_Struct.internalBinaryRead(reader, reader.uint32(), options, message.dbSpecific);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.isolationLevel !== 0)
      writer.tag(2, WireType.Varint).int32(message.isolationLevel);
    for (let i = 0; i < message.queries.length; i++)
      QueryDescriptor.internalBinaryWrite(message.queries[i], writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.params.length; i++)
      QueryParamDescriptor.internalBinaryWrite(message.params[i], writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.groups.length; i++)
      QueryParamGroup.internalBinaryWrite(message.groups[i], writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    if (message.dbSpecific)
      Value_Struct.internalBinaryWrite(message.dbSpecific, writer.tag(6, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var TransactionDescriptor = new TransactionDescriptor$Type();
var UnitDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.UnitDescriptor", [
      { no: 1, name: "create_table", kind: "message", oneof: "type", T: () => TableDescriptor },
      { no: 5, name: "insert", kind: "message", oneof: "type", T: () => InsertDescriptor },
      { no: 2, name: "query", kind: "message", oneof: "type", T: () => QueryDescriptor },
      { no: 4, name: "transaction", kind: "message", oneof: "type", T: () => TransactionDescriptor }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.type = { oneofKind: void 0 };
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.TableDescriptor create_table */
        1:
          message.type = {
            oneofKind: "createTable",
            createTable: TableDescriptor.internalBinaryRead(reader, reader.uint32(), options, message.type.createTable)
          };
          break;
        case /* stroppy.InsertDescriptor insert */
        5:
          message.type = {
            oneofKind: "insert",
            insert: InsertDescriptor.internalBinaryRead(reader, reader.uint32(), options, message.type.insert)
          };
          break;
        case /* stroppy.QueryDescriptor query */
        2:
          message.type = {
            oneofKind: "query",
            query: QueryDescriptor.internalBinaryRead(reader, reader.uint32(), options, message.type.query)
          };
          break;
        case /* stroppy.TransactionDescriptor transaction */
        4:
          message.type = {
            oneofKind: "transaction",
            transaction: TransactionDescriptor.internalBinaryRead(reader, reader.uint32(), options, message.type.transaction)
          };
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.type.oneofKind === "createTable")
      TableDescriptor.internalBinaryWrite(message.type.createTable, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "query")
      QueryDescriptor.internalBinaryWrite(message.type.query, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "transaction")
      TransactionDescriptor.internalBinaryWrite(message.type.transaction, writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    if (message.type.oneofKind === "insert")
      InsertDescriptor.internalBinaryWrite(message.type.insert, writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var UnitDescriptor = new UnitDescriptor$Type();
var WorkloadUnitDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.WorkloadUnitDescriptor", [
      { no: 6, name: "descriptor", kind: "message", T: () => UnitDescriptor },
      {
        no: 5,
        name: "count",
        kind: "scalar",
        T: 4
        /*ScalarType.UINT64*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.count = "0";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.UnitDescriptor descriptor */
        6:
          message.descriptor = UnitDescriptor.internalBinaryRead(reader, reader.uint32(), options, message.descriptor);
          break;
        case /* uint64 count */
        5:
          message.count = reader.uint64().toString();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.count !== "0")
      writer.tag(5, WireType.Varint).uint64(message.count);
    if (message.descriptor)
      UnitDescriptor.internalBinaryWrite(message.descriptor, writer.tag(6, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var WorkloadUnitDescriptor = new WorkloadUnitDescriptor$Type();
var WorkloadDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.WorkloadDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "async",
        kind: "scalar",
        opt: true,
        T: 8
        /*ScalarType.BOOL*/
      },
      { no: 3, name: "units", kind: "message", repeat: 2, T: () => WorkloadUnitDescriptor }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.units = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* optional bool async */
        2:
          message.async = reader.bool();
          break;
        case /* repeated stroppy.WorkloadUnitDescriptor units */
        3:
          message.units.push(WorkloadUnitDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.async !== void 0)
      writer.tag(2, WireType.Varint).bool(message.async);
    for (let i = 0; i < message.units.length; i++)
      WorkloadUnitDescriptor.internalBinaryWrite(message.units[i], writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var WorkloadDescriptor = new WorkloadDescriptor$Type();
var BenchmarkDescriptor$Type = class extends MessageType {
  constructor() {
    super("stroppy.BenchmarkDescriptor", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 100, name: "workloads", kind: "message", repeat: 2, T: () => WorkloadDescriptor }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.workloads = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* repeated stroppy.WorkloadDescriptor workloads */
        100:
          message.workloads.push(WorkloadDescriptor.internalBinaryRead(reader, reader.uint32(), options));
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    for (let i = 0; i < message.workloads.length; i++)
      WorkloadDescriptor.internalBinaryWrite(message.workloads[i], writer.tag(100, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var BenchmarkDescriptor = new BenchmarkDescriptor$Type();

// ts_source/google/protobuf/duration_pb.ts
var Duration$Type = class extends MessageType {
  constructor() {
    super("google.protobuf.Duration", [
      {
        no: 1,
        name: "seconds",
        kind: "scalar",
        T: 3
        /*ScalarType.INT64*/
      },
      {
        no: 2,
        name: "nanos",
        kind: "scalar",
        T: 5
        /*ScalarType.INT32*/
      }
    ]);
  }
  /**
   * Encode `Duration` to JSON string like "3.000001s".
   */
  internalJsonWrite(message, options) {
    let s = PbLong.from(message.seconds).toNumber();
    if (s > 315576e6 || s < -315576e6)
      throw new Error("Duration value out of range.");
    let text = message.seconds.toString();
    if (s === 0 && message.nanos < 0)
      text = "-" + text;
    if (message.nanos !== 0) {
      let nanosStr = Math.abs(message.nanos).toString();
      nanosStr = "0".repeat(9 - nanosStr.length) + nanosStr;
      if (nanosStr.substring(3) === "000000")
        nanosStr = nanosStr.substring(0, 3);
      else if (nanosStr.substring(6) === "000")
        nanosStr = nanosStr.substring(0, 6);
      text += "." + nanosStr;
    }
    return text + "s";
  }
  /**
   * Decode `Duration` from JSON string like "3.000001s"
   */
  internalJsonRead(json, options, target) {
    if (typeof json !== "string")
      throw new Error("Unable to parse Duration from JSON " + typeofJsonValue(json) + ". Expected string.");
    let match = json.match(/^(-?)([0-9]+)(?:\.([0-9]+))?s/);
    if (match === null)
      throw new Error("Unable to parse Duration from JSON string. Invalid format.");
    if (!target)
      target = this.create();
    let [, sign, secs, nanos] = match;
    let longSeconds = PbLong.from(sign + secs);
    if (longSeconds.toNumber() > 315576e6 || longSeconds.toNumber() < -315576e6)
      throw new Error("Unable to parse Duration from JSON string. Value out of range.");
    target.seconds = longSeconds.toString();
    if (typeof nanos == "string") {
      let nanosStr = sign + nanos + "0".repeat(9 - nanos.length);
      target.nanos = parseInt(nanosStr);
    }
    return target;
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.seconds = "0";
    message.nanos = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* int64 seconds */
        1:
          message.seconds = reader.int64().toString();
          break;
        case /* int32 nanos */
        2:
          message.nanos = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.seconds !== "0")
      writer.tag(1, WireType.Varint).int64(message.seconds);
    if (message.nanos !== 0)
      writer.tag(2, WireType.Varint).int32(message.nanos);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Duration = new Duration$Type();

// ts_source/proto/stroppy/k6_pb.ts
var K6Options$Type = class extends MessageType {
  constructor() {
    super("stroppy.K6Options", [
      {
        no: 2,
        name: "k6_args",
        kind: "scalar",
        repeat: 2,
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 10, name: "setup_timeout", kind: "message", T: () => Duration },
      { no: 200, name: "scenario", kind: "message", T: () => K6Scenario }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.k6Args = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* repeated string k6_args */
        2:
          message.k6Args.push(reader.string());
          break;
        case /* optional google.protobuf.Duration setup_timeout */
        10:
          message.setupTimeout = Duration.internalBinaryRead(reader, reader.uint32(), options, message.setupTimeout);
          break;
        case /* stroppy.K6Scenario scenario */
        200:
          message.scenario = K6Scenario.internalBinaryRead(reader, reader.uint32(), options, message.scenario);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    for (let i = 0; i < message.k6Args.length; i++)
      writer.tag(2, WireType.LengthDelimited).string(message.k6Args[i]);
    if (message.setupTimeout)
      Duration.internalBinaryWrite(message.setupTimeout, writer.tag(10, WireType.LengthDelimited).fork(), options).join();
    if (message.scenario)
      K6Scenario.internalBinaryWrite(message.scenario, writer.tag(200, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var K6Options = new K6Options$Type();
var K6Scenario$Type = class extends MessageType {
  constructor() {
    super("stroppy.K6Scenario", [
      { no: 3, name: "max_duration", kind: "message", T: () => Duration },
      { no: 10, name: "shared_iterations", kind: "message", oneof: "executor", T: () => SharedIterations },
      { no: 11, name: "per_vu_iterations", kind: "message", oneof: "executor", T: () => PerVuIterations },
      { no: 12, name: "constant_vus", kind: "message", oneof: "executor", T: () => ConstantVUs },
      { no: 13, name: "ramping_vus", kind: "message", oneof: "executor", T: () => RampingVUs },
      { no: 14, name: "constant_arrival_rate", kind: "message", oneof: "executor", T: () => ConstantArrivalRate },
      { no: 15, name: "ramping_arrival_rate", kind: "message", oneof: "executor", T: () => RampingArrivalRate }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.executor = { oneofKind: void 0 };
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* google.protobuf.Duration max_duration */
        3:
          message.maxDuration = Duration.internalBinaryRead(reader, reader.uint32(), options, message.maxDuration);
          break;
        case /* stroppy.SharedIterations shared_iterations */
        10:
          message.executor = {
            oneofKind: "sharedIterations",
            sharedIterations: SharedIterations.internalBinaryRead(reader, reader.uint32(), options, message.executor.sharedIterations)
          };
          break;
        case /* stroppy.PerVuIterations per_vu_iterations */
        11:
          message.executor = {
            oneofKind: "perVuIterations",
            perVuIterations: PerVuIterations.internalBinaryRead(reader, reader.uint32(), options, message.executor.perVuIterations)
          };
          break;
        case /* stroppy.ConstantVUs constant_vus */
        12:
          message.executor = {
            oneofKind: "constantVus",
            constantVus: ConstantVUs.internalBinaryRead(reader, reader.uint32(), options, message.executor.constantVus)
          };
          break;
        case /* stroppy.RampingVUs ramping_vus */
        13:
          message.executor = {
            oneofKind: "rampingVus",
            rampingVus: RampingVUs.internalBinaryRead(reader, reader.uint32(), options, message.executor.rampingVus)
          };
          break;
        case /* stroppy.ConstantArrivalRate constant_arrival_rate */
        14:
          message.executor = {
            oneofKind: "constantArrivalRate",
            constantArrivalRate: ConstantArrivalRate.internalBinaryRead(reader, reader.uint32(), options, message.executor.constantArrivalRate)
          };
          break;
        case /* stroppy.RampingArrivalRate ramping_arrival_rate */
        15:
          message.executor = {
            oneofKind: "rampingArrivalRate",
            rampingArrivalRate: RampingArrivalRate.internalBinaryRead(reader, reader.uint32(), options, message.executor.rampingArrivalRate)
          };
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.maxDuration)
      Duration.internalBinaryWrite(message.maxDuration, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.executor.oneofKind === "sharedIterations")
      SharedIterations.internalBinaryWrite(message.executor.sharedIterations, writer.tag(10, WireType.LengthDelimited).fork(), options).join();
    if (message.executor.oneofKind === "perVuIterations")
      PerVuIterations.internalBinaryWrite(message.executor.perVuIterations, writer.tag(11, WireType.LengthDelimited).fork(), options).join();
    if (message.executor.oneofKind === "constantVus")
      ConstantVUs.internalBinaryWrite(message.executor.constantVus, writer.tag(12, WireType.LengthDelimited).fork(), options).join();
    if (message.executor.oneofKind === "rampingVus")
      RampingVUs.internalBinaryWrite(message.executor.rampingVus, writer.tag(13, WireType.LengthDelimited).fork(), options).join();
    if (message.executor.oneofKind === "constantArrivalRate")
      ConstantArrivalRate.internalBinaryWrite(message.executor.constantArrivalRate, writer.tag(14, WireType.LengthDelimited).fork(), options).join();
    if (message.executor.oneofKind === "rampingArrivalRate")
      RampingArrivalRate.internalBinaryWrite(message.executor.rampingArrivalRate, writer.tag(15, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var K6Scenario = new K6Scenario$Type();
var SharedIterations$Type = class extends MessageType {
  constructor() {
    super("stroppy.SharedIterations", [
      {
        no: 1,
        name: "iterations",
        kind: "scalar",
        T: 3
        /*ScalarType.INT64*/
      },
      {
        no: 2,
        name: "vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.iterations = "0";
    message.vus = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* int64 iterations */
        1:
          message.iterations = reader.int64().toString();
          break;
        case /* uint32 vus */
        2:
          message.vus = reader.uint32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.iterations !== "0")
      writer.tag(1, WireType.Varint).int64(message.iterations);
    if (message.vus !== 0)
      writer.tag(2, WireType.Varint).uint32(message.vus);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var SharedIterations = new SharedIterations$Type();
var PerVuIterations$Type = class extends MessageType {
  constructor() {
    super("stroppy.PerVuIterations", [
      {
        no: 1,
        name: "vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 2,
        name: "iterations",
        kind: "scalar",
        T: 3
        /*ScalarType.INT64*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.vus = 0;
    message.iterations = "0";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* uint32 vus */
        1:
          message.vus = reader.uint32();
          break;
        case /* int64 iterations */
        2:
          message.iterations = reader.int64().toString();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.vus !== 0)
      writer.tag(1, WireType.Varint).uint32(message.vus);
    if (message.iterations !== "0")
      writer.tag(2, WireType.Varint).int64(message.iterations);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var PerVuIterations = new PerVuIterations$Type();
var ConstantVUs$Type = class extends MessageType {
  constructor() {
    super("stroppy.ConstantVUs", [
      {
        no: 1,
        name: "vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      { no: 2, name: "duration", kind: "message", T: () => Duration }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.vus = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* uint32 vus */
        1:
          message.vus = reader.uint32();
          break;
        case /* google.protobuf.Duration duration */
        2:
          message.duration = Duration.internalBinaryRead(reader, reader.uint32(), options, message.duration);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.vus !== 0)
      writer.tag(1, WireType.Varint).uint32(message.vus);
    if (message.duration)
      Duration.internalBinaryWrite(message.duration, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var ConstantVUs = new ConstantVUs$Type();
var RampingVUs$Type = class extends MessageType {
  constructor() {
    super("stroppy.RampingVUs", [
      {
        no: 1,
        name: "start_vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      { no: 2, name: "stages", kind: "message", repeat: 2, T: () => RampingVUs_VUStage },
      {
        no: 3,
        name: "pre_allocated_vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 4,
        name: "max_vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.startVus = 0;
    message.stages = [];
    message.preAllocatedVus = 0;
    message.maxVus = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* uint32 start_vus */
        1:
          message.startVus = reader.uint32();
          break;
        case /* repeated stroppy.RampingVUs.VUStage stages */
        2:
          message.stages.push(RampingVUs_VUStage.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* uint32 pre_allocated_vus */
        3:
          message.preAllocatedVus = reader.uint32();
          break;
        case /* uint32 max_vus */
        4:
          message.maxVus = reader.uint32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.startVus !== 0)
      writer.tag(1, WireType.Varint).uint32(message.startVus);
    for (let i = 0; i < message.stages.length; i++)
      RampingVUs_VUStage.internalBinaryWrite(message.stages[i], writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.preAllocatedVus !== 0)
      writer.tag(3, WireType.Varint).uint32(message.preAllocatedVus);
    if (message.maxVus !== 0)
      writer.tag(4, WireType.Varint).uint32(message.maxVus);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var RampingVUs = new RampingVUs$Type();
var RampingVUs_VUStage$Type = class extends MessageType {
  constructor() {
    super("stroppy.RampingVUs.VUStage", [
      { no: 1, name: "duration", kind: "message", T: () => Duration },
      {
        no: 2,
        name: "target",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.target = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* google.protobuf.Duration duration */
        1:
          message.duration = Duration.internalBinaryRead(reader, reader.uint32(), options, message.duration);
          break;
        case /* uint32 target */
        2:
          message.target = reader.uint32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.duration)
      Duration.internalBinaryWrite(message.duration, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.target !== 0)
      writer.tag(2, WireType.Varint).uint32(message.target);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var RampingVUs_VUStage = new RampingVUs_VUStage$Type();
var ConstantArrivalRate$Type = class extends MessageType {
  constructor() {
    super("stroppy.ConstantArrivalRate", [
      {
        no: 1,
        name: "rate",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      { no: 2, name: "time_unit", kind: "message", T: () => Duration },
      { no: 3, name: "duration", kind: "message", T: () => Duration },
      {
        no: 4,
        name: "pre_allocated_vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 5,
        name: "max_vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.rate = 0;
    message.preAllocatedVus = 0;
    message.maxVus = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* uint32 rate */
        1:
          message.rate = reader.uint32();
          break;
        case /* google.protobuf.Duration time_unit */
        2:
          message.timeUnit = Duration.internalBinaryRead(reader, reader.uint32(), options, message.timeUnit);
          break;
        case /* google.protobuf.Duration duration */
        3:
          message.duration = Duration.internalBinaryRead(reader, reader.uint32(), options, message.duration);
          break;
        case /* uint32 pre_allocated_vus */
        4:
          message.preAllocatedVus = reader.uint32();
          break;
        case /* uint32 max_vus */
        5:
          message.maxVus = reader.uint32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.rate !== 0)
      writer.tag(1, WireType.Varint).uint32(message.rate);
    if (message.timeUnit)
      Duration.internalBinaryWrite(message.timeUnit, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.duration)
      Duration.internalBinaryWrite(message.duration, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.preAllocatedVus !== 0)
      writer.tag(4, WireType.Varint).uint32(message.preAllocatedVus);
    if (message.maxVus !== 0)
      writer.tag(5, WireType.Varint).uint32(message.maxVus);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var ConstantArrivalRate = new ConstantArrivalRate$Type();
var RampingArrivalRate$Type = class extends MessageType {
  constructor() {
    super("stroppy.RampingArrivalRate", [
      {
        no: 1,
        name: "start_rate",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      { no: 2, name: "time_unit", kind: "message", T: () => Duration },
      { no: 3, name: "stages", kind: "message", repeat: 2, T: () => RampingArrivalRate_RateStage },
      {
        no: 4,
        name: "pre_allocated_vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      {
        no: 5,
        name: "max_vus",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.startRate = 0;
    message.stages = [];
    message.preAllocatedVus = 0;
    message.maxVus = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* uint32 start_rate */
        1:
          message.startRate = reader.uint32();
          break;
        case /* google.protobuf.Duration time_unit */
        2:
          message.timeUnit = Duration.internalBinaryRead(reader, reader.uint32(), options, message.timeUnit);
          break;
        case /* repeated stroppy.RampingArrivalRate.RateStage stages */
        3:
          message.stages.push(RampingArrivalRate_RateStage.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* uint32 pre_allocated_vus */
        4:
          message.preAllocatedVus = reader.uint32();
          break;
        case /* uint32 max_vus */
        5:
          message.maxVus = reader.uint32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.startRate !== 0)
      writer.tag(1, WireType.Varint).uint32(message.startRate);
    if (message.timeUnit)
      Duration.internalBinaryWrite(message.timeUnit, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.stages.length; i++)
      RampingArrivalRate_RateStage.internalBinaryWrite(message.stages[i], writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.preAllocatedVus !== 0)
      writer.tag(4, WireType.Varint).uint32(message.preAllocatedVus);
    if (message.maxVus !== 0)
      writer.tag(5, WireType.Varint).uint32(message.maxVus);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var RampingArrivalRate = new RampingArrivalRate$Type();
var RampingArrivalRate_RateStage$Type = class extends MessageType {
  constructor() {
    super("stroppy.RampingArrivalRate.RateStage", [
      {
        no: 1,
        name: "target",
        kind: "scalar",
        T: 13
        /*ScalarType.UINT32*/
      },
      { no: 2, name: "duration", kind: "message", T: () => Duration }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.target = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* uint32 target */
        1:
          message.target = reader.uint32();
          break;
        case /* google.protobuf.Duration duration */
        2:
          message.duration = Duration.internalBinaryRead(reader, reader.uint32(), options, message.duration);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.target !== 0)
      writer.tag(1, WireType.Varint).uint32(message.target);
    if (message.duration)
      Duration.internalBinaryWrite(message.duration, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var RampingArrivalRate_RateStage = new RampingArrivalRate_RateStage$Type();

// ts_source/proto/stroppy/config_pb.ts
var DriverConfig_DriverType = /* @__PURE__ */ ((DriverConfig_DriverType2) => {
  DriverConfig_DriverType2[DriverConfig_DriverType2["DRIVER_TYPE_UNSPECIFIED"] = 0] = "DRIVER_TYPE_UNSPECIFIED";
  DriverConfig_DriverType2[DriverConfig_DriverType2["DRIVER_TYPE_POSTGRES"] = 1] = "DRIVER_TYPE_POSTGRES";
  return DriverConfig_DriverType2;
})(DriverConfig_DriverType || {});
var LoggerConfig_LogLevel = /* @__PURE__ */ ((LoggerConfig_LogLevel2) => {
  LoggerConfig_LogLevel2[LoggerConfig_LogLevel2["LOG_LEVEL_DEBUG"] = 0] = "LOG_LEVEL_DEBUG";
  LoggerConfig_LogLevel2[LoggerConfig_LogLevel2["LOG_LEVEL_INFO"] = 1] = "LOG_LEVEL_INFO";
  LoggerConfig_LogLevel2[LoggerConfig_LogLevel2["LOG_LEVEL_WARN"] = 2] = "LOG_LEVEL_WARN";
  LoggerConfig_LogLevel2[LoggerConfig_LogLevel2["LOG_LEVEL_ERROR"] = 3] = "LOG_LEVEL_ERROR";
  LoggerConfig_LogLevel2[LoggerConfig_LogLevel2["LOG_LEVEL_FATAL"] = 4] = "LOG_LEVEL_FATAL";
  return LoggerConfig_LogLevel2;
})(LoggerConfig_LogLevel || {});
var LoggerConfig_LogMode = /* @__PURE__ */ ((LoggerConfig_LogMode2) => {
  LoggerConfig_LogMode2[LoggerConfig_LogMode2["LOG_MODE_DEVELOPMENT"] = 0] = "LOG_MODE_DEVELOPMENT";
  LoggerConfig_LogMode2[LoggerConfig_LogMode2["LOG_MODE_PRODUCTION"] = 1] = "LOG_MODE_PRODUCTION";
  return LoggerConfig_LogMode2;
})(LoggerConfig_LogMode || {});
var DriverConfig$Type = class extends MessageType {
  constructor() {
    super("stroppy.DriverConfig", [
      {
        no: 1,
        name: "url",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 2, name: "db_specific", kind: "message", T: () => Value_Struct },
      { no: 3, name: "driver_type", kind: "enum", T: () => ["stroppy.DriverConfig.DriverType", DriverConfig_DriverType] }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.url = "";
    message.driverType = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string url */
        1:
          message.url = reader.string();
          break;
        case /* optional stroppy.Value.Struct db_specific */
        2:
          message.dbSpecific = Value_Struct.internalBinaryRead(reader, reader.uint32(), options, message.dbSpecific);
          break;
        case /* stroppy.DriverConfig.DriverType driver_type */
        3:
          message.driverType = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.url !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.url);
    if (message.dbSpecific)
      Value_Struct.internalBinaryWrite(message.dbSpecific, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.driverType !== 0)
      writer.tag(3, WireType.Varint).int32(message.driverType);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var DriverConfig = new DriverConfig$Type();
var LoggerConfig$Type = class extends MessageType {
  constructor() {
    super("stroppy.LoggerConfig", [
      { no: 1, name: "log_level", kind: "enum", T: () => ["stroppy.LoggerConfig.LogLevel", LoggerConfig_LogLevel] },
      { no: 2, name: "log_mode", kind: "enum", T: () => ["stroppy.LoggerConfig.LogMode", LoggerConfig_LogMode] }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.logLevel = 0;
    message.logMode = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.LoggerConfig.LogLevel log_level */
        1:
          message.logLevel = reader.int32();
          break;
        case /* stroppy.LoggerConfig.LogMode log_mode */
        2:
          message.logMode = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.logLevel !== 0)
      writer.tag(1, WireType.Varint).int32(message.logLevel);
    if (message.logMode !== 0)
      writer.tag(2, WireType.Varint).int32(message.logMode);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var LoggerConfig = new LoggerConfig$Type();
var ExporterConfig$Type = class extends MessageType {
  constructor() {
    super("stroppy.ExporterConfig", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 2, name: "otlp_export", kind: "message", T: () => OtlpExport }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* stroppy.OtlpExport otlp_export */
        2:
          message.otlpExport = OtlpExport.internalBinaryRead(reader, reader.uint32(), options, message.otlpExport);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.otlpExport)
      OtlpExport.internalBinaryWrite(message.otlpExport, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var ExporterConfig = new ExporterConfig$Type();
var ExecutorConfig$Type = class extends MessageType {
  constructor() {
    super("stroppy.ExecutorConfig", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 2, name: "k6", kind: "message", T: () => K6Options }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* stroppy.K6Options k6 */
        2:
          message.k6 = K6Options.internalBinaryRead(reader, reader.uint32(), options, message.k6);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.k6)
      K6Options.internalBinaryWrite(message.k6, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var ExecutorConfig = new ExecutorConfig$Type();
var Step$Type = class extends MessageType {
  constructor() {
    super("stroppy.Step", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "workload",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 3,
        name: "executor",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 4,
        name: "exporter",
        kind: "scalar",
        opt: true,
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.workload = "";
    message.executor = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* string workload */
        2:
          message.workload = reader.string();
          break;
        case /* string executor */
        3:
          message.executor = reader.string();
          break;
        case /* optional string exporter */
        4:
          message.exporter = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.workload !== "")
      writer.tag(2, WireType.LengthDelimited).string(message.workload);
    if (message.executor !== "")
      writer.tag(3, WireType.LengthDelimited).string(message.executor);
    if (message.exporter !== void 0)
      writer.tag(4, WireType.LengthDelimited).string(message.exporter);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var Step = new Step$Type();
var SideCarConfig$Type = class extends MessageType {
  constructor() {
    super("stroppy.SideCarConfig", [
      {
        no: 2,
        name: "url",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 3, name: "settings", kind: "message", T: () => Value_Struct }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.url = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string url */
        2:
          message.url = reader.string();
          break;
        case /* optional stroppy.Value.Struct settings */
        3:
          message.settings = Value_Struct.internalBinaryRead(reader, reader.uint32(), options, message.settings);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.url !== "")
      writer.tag(2, WireType.LengthDelimited).string(message.url);
    if (message.settings)
      Value_Struct.internalBinaryWrite(message.settings, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var SideCarConfig = new SideCarConfig$Type();
var CloudConfig$Type = class extends MessageType {
  constructor() {
    super("stroppy.CloudConfig", []);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var CloudConfig = new CloudConfig$Type();
var GlobalConfig$Type = class extends MessageType {
  constructor() {
    super("stroppy.GlobalConfig", [
      {
        no: 1,
        name: "version",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "run_id",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 3,
        name: "seed",
        kind: "scalar",
        T: 4
        /*ScalarType.UINT64*/
      },
      { no: 4, name: "metadata", kind: "map", K: 9, V: {
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      } },
      { no: 5, name: "driver", kind: "message", T: () => DriverConfig },
      { no: 6, name: "logger", kind: "message", T: () => LoggerConfig },
      { no: 7, name: "exporter", kind: "message", T: () => ExporterConfig }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.version = "";
    message.runId = "";
    message.seed = "0";
    message.metadata = {};
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string version */
        1:
          message.version = reader.string();
          break;
        case /* string run_id */
        2:
          message.runId = reader.string();
          break;
        case /* uint64 seed */
        3:
          message.seed = reader.uint64().toString();
          break;
        case /* map<string, string> metadata */
        4:
          this.binaryReadMap4(message.metadata, reader, options);
          break;
        case /* stroppy.DriverConfig driver */
        5:
          message.driver = DriverConfig.internalBinaryRead(reader, reader.uint32(), options, message.driver);
          break;
        case /* stroppy.LoggerConfig logger */
        6:
          message.logger = LoggerConfig.internalBinaryRead(reader, reader.uint32(), options, message.logger);
          break;
        case /* stroppy.ExporterConfig exporter */
        7:
          message.exporter = ExporterConfig.internalBinaryRead(reader, reader.uint32(), options, message.exporter);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  binaryReadMap4(map, reader, options) {
    let len = reader.uint32(), end = reader.pos + len, key, val;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case 1:
          key = reader.string();
          break;
        case 2:
          val = reader.string();
          break;
        default:
          throw new globalThis.Error("unknown map entry field for stroppy.GlobalConfig.metadata");
      }
    }
    map[key ?? ""] = val ?? "";
  }
  internalBinaryWrite(message, writer, options) {
    if (message.version !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.version);
    if (message.runId !== "")
      writer.tag(2, WireType.LengthDelimited).string(message.runId);
    if (message.seed !== "0")
      writer.tag(3, WireType.Varint).uint64(message.seed);
    for (let k of globalThis.Object.keys(message.metadata))
      writer.tag(4, WireType.LengthDelimited).fork().tag(1, WireType.LengthDelimited).string(k).tag(2, WireType.LengthDelimited).string(message.metadata[k]).join();
    if (message.driver)
      DriverConfig.internalBinaryWrite(message.driver, writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    if (message.logger)
      LoggerConfig.internalBinaryWrite(message.logger, writer.tag(6, WireType.LengthDelimited).fork(), options).join();
    if (message.exporter)
      ExporterConfig.internalBinaryWrite(message.exporter, writer.tag(7, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var GlobalConfig = new GlobalConfig$Type();
var ConfigFile$Type = class extends MessageType {
  constructor() {
    super("stroppy.ConfigFile", [
      { no: 1, name: "global", kind: "message", T: () => GlobalConfig },
      { no: 2, name: "exporters", kind: "message", repeat: 2, T: () => ExporterConfig },
      { no: 3, name: "executors", kind: "message", repeat: 2, T: () => ExecutorConfig },
      { no: 4, name: "steps", kind: "message", repeat: 2, T: () => Step },
      { no: 5, name: "side_cars", kind: "message", repeat: 2, T: () => SideCarConfig },
      { no: 6, name: "benchmark", kind: "message", T: () => BenchmarkDescriptor }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.exporters = [];
    message.executors = [];
    message.steps = [];
    message.sideCars = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.GlobalConfig global */
        1:
          message.global = GlobalConfig.internalBinaryRead(reader, reader.uint32(), options, message.global);
          break;
        case /* repeated stroppy.ExporterConfig exporters */
        2:
          message.exporters.push(ExporterConfig.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* repeated stroppy.ExecutorConfig executors */
        3:
          message.executors.push(ExecutorConfig.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* repeated stroppy.Step steps */
        4:
          message.steps.push(Step.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* repeated stroppy.SideCarConfig side_cars */
        5:
          message.sideCars.push(SideCarConfig.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* stroppy.BenchmarkDescriptor benchmark */
        6:
          message.benchmark = BenchmarkDescriptor.internalBinaryRead(reader, reader.uint32(), options, message.benchmark);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.global)
      GlobalConfig.internalBinaryWrite(message.global, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.exporters.length; i++)
      ExporterConfig.internalBinaryWrite(message.exporters[i], writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.executors.length; i++)
      ExecutorConfig.internalBinaryWrite(message.executors[i], writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.steps.length; i++)
      Step.internalBinaryWrite(message.steps[i], writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    for (let i = 0; i < message.sideCars.length; i++)
      SideCarConfig.internalBinaryWrite(message.sideCars[i], writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    if (message.benchmark)
      BenchmarkDescriptor.internalBinaryWrite(message.benchmark, writer.tag(6, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var ConfigFile = new ConfigFile$Type();

// ts_source/proto/stroppy/runtime_pb.ts
var StepContext$Type = class extends MessageType {
  constructor() {
    super("stroppy.StepContext", [
      { no: 1, name: "config", kind: "message", T: () => GlobalConfig },
      { no: 2, name: "step", kind: "message", T: () => Step },
      { no: 3, name: "executor", kind: "message", T: () => ExecutorConfig },
      { no: 4, name: "exporter", kind: "message", T: () => ExporterConfig },
      { no: 5, name: "workload", kind: "message", T: () => WorkloadDescriptor }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.GlobalConfig config */
        1:
          message.config = GlobalConfig.internalBinaryRead(reader, reader.uint32(), options, message.config);
          break;
        case /* stroppy.Step step */
        2:
          message.step = Step.internalBinaryRead(reader, reader.uint32(), options, message.step);
          break;
        case /* stroppy.ExecutorConfig executor */
        3:
          message.executor = ExecutorConfig.internalBinaryRead(reader, reader.uint32(), options, message.executor);
          break;
        case /* optional stroppy.ExporterConfig exporter */
        4:
          message.exporter = ExporterConfig.internalBinaryRead(reader, reader.uint32(), options, message.exporter);
          break;
        case /* stroppy.WorkloadDescriptor workload */
        5:
          message.workload = WorkloadDescriptor.internalBinaryRead(reader, reader.uint32(), options, message.workload);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.config)
      GlobalConfig.internalBinaryWrite(message.config, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.step)
      Step.internalBinaryWrite(message.step, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.executor)
      ExecutorConfig.internalBinaryWrite(message.executor, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.exporter)
      ExporterConfig.internalBinaryWrite(message.exporter, writer.tag(4, WireType.LengthDelimited).fork(), options).join();
    if (message.workload)
      WorkloadDescriptor.internalBinaryWrite(message.workload, writer.tag(5, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var StepContext = new StepContext$Type();
var UnitContext$Type = class extends MessageType {
  constructor() {
    super("stroppy.UnitContext", [
      { no: 1, name: "step_context", kind: "message", T: () => StepContext },
      { no: 2, name: "unit_descriptor", kind: "message", T: () => WorkloadUnitDescriptor }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.StepContext step_context */
        1:
          message.stepContext = StepContext.internalBinaryRead(reader, reader.uint32(), options, message.stepContext);
          break;
        case /* stroppy.WorkloadUnitDescriptor unit_descriptor */
        2:
          message.unitDescriptor = WorkloadUnitDescriptor.internalBinaryRead(reader, reader.uint32(), options, message.unitDescriptor);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.stepContext)
      StepContext.internalBinaryWrite(message.stepContext, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.unitDescriptor)
      WorkloadUnitDescriptor.internalBinaryWrite(message.unitDescriptor, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var UnitContext = new UnitContext$Type();
var DriverQuery$Type = class extends MessageType {
  constructor() {
    super("stroppy.DriverQuery", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      {
        no: 2,
        name: "request",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 3, name: "params", kind: "message", repeat: 2, T: () => Value },
      { no: 4, name: "method", kind: "enum", opt: true, T: () => ["stroppy.InsertMethod", InsertMethod] }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    message.request = "";
    message.params = [];
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* string request */
        2:
          message.request = reader.string();
          break;
        case /* repeated stroppy.Value params */
        3:
          message.params.push(Value.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* optional stroppy.InsertMethod method */
        4:
          message.method = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.request !== "")
      writer.tag(2, WireType.LengthDelimited).string(message.request);
    for (let i = 0; i < message.params.length; i++)
      Value.internalBinaryWrite(message.params[i], writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.method !== void 0)
      writer.tag(4, WireType.Varint).int32(message.method);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var DriverQuery = new DriverQuery$Type();
var DriverTransaction$Type = class extends MessageType {
  constructor() {
    super("stroppy.DriverTransaction", [
      { no: 1, name: "queries", kind: "message", repeat: 2, T: () => DriverQuery },
      { no: 2, name: "isolation_level", kind: "enum", T: () => ["stroppy.TxIsolationLevel", TxIsolationLevel] }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.queries = [];
    message.isolationLevel = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* repeated stroppy.DriverQuery queries */
        1:
          message.queries.push(DriverQuery.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* stroppy.TxIsolationLevel isolation_level */
        2:
          message.isolationLevel = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    for (let i = 0; i < message.queries.length; i++)
      DriverQuery.internalBinaryWrite(message.queries[i], writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.isolationLevel !== 0)
      writer.tag(2, WireType.Varint).int32(message.isolationLevel);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var DriverTransaction = new DriverTransaction$Type();
var DriverQueryStat$Type = class extends MessageType {
  constructor() {
    super("stroppy.DriverQueryStat", [
      {
        no: 1,
        name: "name",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      },
      { no: 2, name: "exec_duration", kind: "message", T: () => Duration }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.name = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* string name */
        1:
          message.name = reader.string();
          break;
        case /* google.protobuf.Duration exec_duration */
        2:
          message.execDuration = Duration.internalBinaryRead(reader, reader.uint32(), options, message.execDuration);
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.name !== "")
      writer.tag(1, WireType.LengthDelimited).string(message.name);
    if (message.execDuration)
      Duration.internalBinaryWrite(message.execDuration, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var DriverQueryStat = new DriverQueryStat$Type();
var DriverTransactionStat$Type = class extends MessageType {
  constructor() {
    super("stroppy.DriverTransactionStat", [
      { no: 1, name: "queries", kind: "message", repeat: 2, T: () => DriverQueryStat },
      { no: 2, name: "exec_duration", kind: "message", T: () => Duration },
      { no: 3, name: "isolation_level", kind: "enum", T: () => ["stroppy.TxIsolationLevel", TxIsolationLevel] }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.queries = [];
    message.isolationLevel = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* repeated stroppy.DriverQueryStat queries */
        1:
          message.queries.push(DriverQueryStat.internalBinaryRead(reader, reader.uint32(), options));
          break;
        case /* google.protobuf.Duration exec_duration */
        2:
          message.execDuration = Duration.internalBinaryRead(reader, reader.uint32(), options, message.execDuration);
          break;
        case /* stroppy.TxIsolationLevel isolation_level */
        3:
          message.isolationLevel = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    for (let i = 0; i < message.queries.length; i++)
      DriverQueryStat.internalBinaryWrite(message.queries[i], writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.execDuration)
      Duration.internalBinaryWrite(message.execDuration, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.isolationLevel !== 0)
      writer.tag(3, WireType.Varint).int32(message.isolationLevel);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var DriverTransactionStat = new DriverTransactionStat$Type();

// ts_source/proto/stroppy/cloud_pb.ts
var Status = /* @__PURE__ */ ((Status2) => {
  Status2[Status2["STATUS_IDLE"] = 0] = "STATUS_IDLE";
  Status2[Status2["STATUS_RUNNING"] = 1] = "STATUS_RUNNING";
  Status2[Status2["STATUS_COMPLETED"] = 2] = "STATUS_COMPLETED";
  Status2[Status2["STATUS_FAILED"] = 3] = "STATUS_FAILED";
  Status2[Status2["STATUS_CANCELLED"] = 4] = "STATUS_CANCELLED";
  return Status2;
})(Status || {});
var StroppyStepRun$Type = class extends MessageType {
  constructor() {
    super("stroppy.StroppyStepRun", [
      { no: 1, name: "id", kind: "message", T: () => Ulid },
      { no: 2, name: "stroppy_run_id", kind: "message", T: () => Ulid },
      { no: 3, name: "context", kind: "message", T: () => StepContext },
      { no: 4, name: "status", kind: "enum", T: () => ["stroppy.Status", Status] }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.status = 0;
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.Ulid id */
        1:
          message.id = Ulid.internalBinaryRead(reader, reader.uint32(), options, message.id);
          break;
        case /* stroppy.Ulid stroppy_run_id */
        2:
          message.stroppyRunId = Ulid.internalBinaryRead(reader, reader.uint32(), options, message.stroppyRunId);
          break;
        case /* stroppy.StepContext context */
        3:
          message.context = StepContext.internalBinaryRead(reader, reader.uint32(), options, message.context);
          break;
        case /* stroppy.Status status */
        4:
          message.status = reader.int32();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.id)
      Ulid.internalBinaryWrite(message.id, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.stroppyRunId)
      Ulid.internalBinaryWrite(message.stroppyRunId, writer.tag(2, WireType.LengthDelimited).fork(), options).join();
    if (message.context)
      StepContext.internalBinaryWrite(message.context, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.status !== 0)
      writer.tag(4, WireType.Varint).int32(message.status);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var StroppyStepRun = new StroppyStepRun$Type();
var StroppyRun$Type = class extends MessageType {
  constructor() {
    super("stroppy.StroppyRun", [
      { no: 1, name: "id", kind: "message", T: () => Ulid },
      { no: 2, name: "status", kind: "enum", T: () => ["stroppy.Status", Status] },
      { no: 3, name: "config", kind: "message", T: () => ConfigFile },
      {
        no: 4,
        name: "cmd",
        kind: "scalar",
        T: 9
        /*ScalarType.STRING*/
      }
    ]);
  }
  create(value) {
    const message = globalThis.Object.create(this.messagePrototype);
    message.status = 0;
    message.cmd = "";
    if (value !== void 0)
      reflectionMergePartial(this, message, value);
    return message;
  }
  internalBinaryRead(reader, length, options, target) {
    let message = target ?? this.create(), end = reader.pos + length;
    while (reader.pos < end) {
      let [fieldNo, wireType] = reader.tag();
      switch (fieldNo) {
        case /* stroppy.Ulid id */
        1:
          message.id = Ulid.internalBinaryRead(reader, reader.uint32(), options, message.id);
          break;
        case /* stroppy.Status status */
        2:
          message.status = reader.int32();
          break;
        case /* stroppy.ConfigFile config */
        3:
          message.config = ConfigFile.internalBinaryRead(reader, reader.uint32(), options, message.config);
          break;
        case /* string cmd */
        4:
          message.cmd = reader.string();
          break;
        default:
          let u = options.readUnknownField;
          if (u === "throw")
            throw new globalThis.Error(`Unknown field ${fieldNo} (wire type ${wireType}) for ${this.typeName}`);
          let d = reader.skip(wireType);
          if (u !== false)
            (u === true ? UnknownFieldHandler.onRead : u)(this.typeName, message, fieldNo, wireType, d);
      }
    }
    return message;
  }
  internalBinaryWrite(message, writer, options) {
    if (message.id)
      Ulid.internalBinaryWrite(message.id, writer.tag(1, WireType.LengthDelimited).fork(), options).join();
    if (message.status !== 0)
      writer.tag(2, WireType.Varint).int32(message.status);
    if (message.config)
      ConfigFile.internalBinaryWrite(message.config, writer.tag(3, WireType.LengthDelimited).fork(), options).join();
    if (message.cmd !== "")
      writer.tag(4, WireType.LengthDelimited).string(message.cmd);
    let u = options.writeUnknownFields;
    if (u !== false)
      (u == true ? UnknownFieldHandler.onWrite : u)(this.typeName, message, writer);
    return writer;
  }
};
var StroppyRun = new StroppyRun$Type();
export {
  BenchmarkDescriptor,
  CloudConfig,
  ColumnDescriptor,
  ConfigFile,
  ConstantArrivalRate,
  ConstantVUs,
  DateTime,
  Decimal,
  DriverConfig,
  DriverConfig_DriverType,
  DriverQuery,
  DriverQueryStat,
  DriverTransaction,
  DriverTransactionStat,
  ExecutorConfig,
  ExporterConfig,
  Generation,
  Generation_Alphabet,
  Generation_Distribution,
  Generation_Distribution_DistributionType,
  Generation_Range,
  Generation_Range_AnyString,
  Generation_Range_Bool,
  Generation_Range_DateTime,
  Generation_Range_DateTime_TimestampPb,
  Generation_Range_DateTime_TimestampUnix,
  Generation_Range_DecimalRange,
  Generation_Range_Double,
  Generation_Range_Float,
  Generation_Range_Int32,
  Generation_Range_Int64,
  Generation_Range_String,
  Generation_Range_UInt32,
  Generation_Range_UInt64,
  Generation_Rule,
  GlobalConfig,
  IndexDescriptor,
  InsertDescriptor,
  InsertMethod,
  K6Options,
  K6Scenario,
  LoggerConfig,
  LoggerConfig_LogLevel,
  LoggerConfig_LogMode,
  OtlpExport,
  PerVuIterations,
  QueryDescriptor,
  QueryParamDescriptor,
  QueryParamGroup,
  RampingArrivalRate,
  RampingArrivalRate_RateStage,
  RampingVUs,
  RampingVUs_VUStage,
  SharedIterations,
  SideCarConfig,
  Status,
  Step,
  StepContext,
  StroppyRun,
  StroppyStepRun,
  TableDescriptor,
  TransactionDescriptor,
  TxIsolationLevel,
  Ulid,
  UnitContext,
  UnitDescriptor,
  Uuid,
  Value,
  Value_List,
  Value_NullValue,
  Value_Struct,
  WorkloadDescriptor,
  WorkloadUnitDescriptor
};
