import * as fs from "fs";
import { ConfigFile } from "./stroppy.pb";
import serialize from "serialize-javascript";

const jsonPath = process.argv[2];
if (!jsonPath) {
  console.error("Usage: ts-node json_to_ts_config.ts <json_file>");
  process.exit(1);
}

function toTsLiteral(obj) {
  return serialize(obj, { space: 2 });
}

try {
  const content = fs.readFileSync(jsonPath, "utf8");
  const json = JSON.parse(content);
  delete json["$schema"];
  const config = ConfigFile.fromJson(json);
  const literal = toTsLiteral(config);
  const output = `import { ConfigFile } from './stroppy.pb';\nconst config: ConfigFile = ${literal};\n`;
  fs.writeFileSync("translated.ts", output);
} catch (error) {
  console.error("Error processing JSON:", error.message);
  process.exit(1);
}
