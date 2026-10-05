'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const {spawnSync} = require('node:child_process');

for (const recordFlag of ['--record', '--record-v011']) {
  for (const hasBaseline of [true, false]) {
    test(`record refusal leaves artifacts unchanged (baseline=${hasBaseline}, flag=${recordFlag})`, t => {
      const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'protocarry-record-'));
      t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
      fs.copyFileSync(path.join(__dirname, 'validate.cjs'), path.join(dir, 'validate.cjs'));
      const evidence = 'evidence-v0.1.1';
      const results = 'results-v0.1.1.json';
      fs.mkdirSync(path.join(dir, 'evidence'));
      fs.writeFileSync(path.join(dir, 'evidence', 'legacy.bin'), 'legacy fixture');
      fs.writeFileSync(path.join(dir, 'results.json'), 'legacy measurement');
      fs.mkdirSync(path.join(dir, evidence));
      fs.writeFileSync(path.join(dir, evidence, 'keep.bin'), Buffer.from([0, 1, 255]));
      const baseline = path.join(dir, results);
      const original = '{"existing":"baseline"}\n';
      if (hasBaseline) fs.writeFileSync(baseline, original);

      // No CLI or npm runtime is installed here: rejection must precede measurement.
      const result = spawnSync(process.execPath, [path.join(dir, 'validate.cjs'), recordFlag],
        {encoding:'utf8', timeout:5000});
      assert.ifError(result.error);
      assert.equal(result.status, 1);
      assert.match(result.stderr, /committed evidence destination exists/);
      assert.equal(fs.existsSync(path.join(dir, 'runs')), false);
      assert.deepEqual(fs.readdirSync(path.join(dir, evidence)), ['keep.bin']);
      assert.deepEqual(fs.readFileSync(path.join(dir, evidence, 'keep.bin')), Buffer.from([0, 1, 255]));
      assert.equal(fs.existsSync(baseline), hasBaseline);
      if (hasBaseline) assert.equal(fs.readFileSync(baseline, 'utf8'), original);
      assert.equal(fs.readFileSync(path.join(dir, 'evidence', 'legacy.bin'), 'utf8'), 'legacy fixture');
      assert.equal(fs.readFileSync(path.join(dir, 'results.json'), 'utf8'), 'legacy measurement');
    });
  }
}

test('unknown option is rejected before creating a run', t => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'protocarry-option-'));
  t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
  fs.copyFileSync(path.join(__dirname, 'validate.cjs'), path.join(dir, 'validate.cjs'));
  const result = spawnSync(process.execPath, [path.join(dir, 'validate.cjs'), '--recrod'],
    {encoding:'utf8', timeout:5000});
  assert.ifError(result.error);
  assert.equal(result.status, 1);
  assert.match(result.stderr, /usage: node validate.cjs/);
  assert.equal(fs.existsSync(path.join(dir, 'runs')), false);
});

for (const recordFlag of ['--record', '--record-v011']) {
  test(`legacy evidence does not block recording current measurements (${recordFlag})`, t => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'protocarry-legacy-'));
    t.after(() => fs.rmSync(dir, {recursive:true, force:true}));
    fs.copyFileSync(path.join(__dirname, 'validate.cjs'), path.join(dir, 'validate.cjs'));
    fs.mkdirSync(path.join(dir, 'evidence'));
    fs.writeFileSync(path.join(dir, 'evidence', 'legacy.bin'), 'legacy fixture');
    fs.writeFileSync(path.join(dir, 'results.json'), 'legacy measurement');
    // Measurement cannot finish without the CLI/runtime fixtures, but preflight
    // must accept existing legacy evidence and leave it untouched.
    const result = spawnSync(process.execPath, [path.join(dir, 'validate.cjs'), recordFlag],
      {encoding:'utf8', timeout:5000});
    assert.ifError(result.error);
    assert.equal(result.status, 1);
    assert.doesNotMatch(result.stderr, /committed evidence destination exists/);
    assert.equal(fs.existsSync(path.join(dir, 'runs')), true);
    assert.equal(fs.existsSync(path.join(dir, 'evidence-v0.1.1')), false);
    assert.equal(fs.readFileSync(path.join(dir, 'evidence', 'legacy.bin'), 'utf8'), 'legacy fixture');
    assert.equal(fs.readFileSync(path.join(dir, 'results.json'), 'utf8'), 'legacy measurement');
  });
}
