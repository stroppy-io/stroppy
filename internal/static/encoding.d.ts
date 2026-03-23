// Type declarations for k6/x/encoding module (xk6-encoding extension)

declare module "k6/x/encoding" {
  const encoding: {
    TextEncoder: typeof TextEncoder;
    TextDecoder: typeof TextDecoder;
  };
  export default encoding;
}
