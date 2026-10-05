'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

for (const hasBaseline of [true, false]) {
  test(`record refusal leaves artifacts unchanged (baseline=${hasBaseline})`, t => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'protocarry-record-'));
    t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
    fs.copyFileSync(path.join(__dirname, 'validate.cjs'), path.join(dir, 'validate.cjs'));
    fs.mkdirSync(path.join(dir, 'evidence'));
    fs.writeFileSync(path.join(dir, 'evidence', 'keep.bin'), Buffer.from([0, 1, 255]));
    const baseline = path.join(dir, 'results.json');
    const original = '{"existing":"baseline"}\n';
    if (hasBaseline) fs.writeFileSync(baseline, original);

    // No CLI or npm runtime is installed here: rejection must precede measurement.
    const result = spawnSync(process.execPath, [path.join(dir, 'validate.cjs'), '--record'],
      {encoding:'utf8', timeout:5000});
    assert.ifError(result.error);
    assert.equal(result.status, 1);
    assert.match(result.stderr, /committed evidence destination exists/);
    assert.equal(fs.existsSync(path.join(dir, 'runs')), false);
    assert.deepEqual(fs.readdirSync(path.join(dir, 'evidence')), ['keep.bin']);
    assert.deepEqual(fs.readFileSync(path.join(dir, 'evidence', 'keep.bin')), Buffer.from([0, 1, 255]));
    assert.equal(fs.existsSync(baseline), hasBaseline);
    if (hasBaseline) assert.equal(fs.readFileSync(baseline, 'utf8'), original);
  });
}
