'use strict';
// Real upstream binary decode/encode using the old application's schema.
// No preservation assertion, new schema or oracle is available to this adapter.
const fs = require('node:fs');
const path = require('node:path');
const [alias, option, name] = process.argv.slice(2);
if (!['pb861', 'pb862', 'pb880'].includes(alias) || !['default', 'preserve'].includes(option)
    || !['validation.Envelope', 'validation.NestedEnvelope'].includes(name)) {
  console.error('usage: node relay.cjs pb861|pb862|pb880 default|preserve MESSAGE');
  process.exit(64);
}
const pb = require(alias);
const version = require(`${alias}/package.json`).version;
const type = pb.loadSync(path.join(__dirname, 'old.proto')).lookupType(name);
const input = fs.readFileSync(0);
const reader = pb.Reader.create(input);
if (option === 'preserve') reader.discardUnknown = false;
console.error(JSON.stringify({runtime: 'protobuf.js', version, node: process.version,
  option, readerDiscardUnknown: reader.discardUnknown ?? null}));
const decoded = type.decode(reader);
process.stdout.write(type.encode(decoded).finish());
