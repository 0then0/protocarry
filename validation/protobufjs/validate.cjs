'use strict';
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const {spawnSync} = require('node:child_process');
const root = path.resolve(__dirname, '../..');
const cli = path.resolve(process.env.PROTOCARRY_BIN || path.join(root, 'bin/protocarry'));
const goRelay = path.resolve(process.env.PROTOCARRY_GO_RELAY || path.join(root, 'bin/demo-adapter'));
const trials = 3;
const runs = path.join(__dirname, 'runs');
if (process.argv.slice(2).some(arg => !['--record', '--record-v011'].includes(arg)))
  throw new Error('usage: node validate.cjs [--record]');
// Retain the former spelling as an alias; neither option replaces legacy data.
const record = process.argv.includes('--record') || process.argv.includes('--record-v011');
const destination = path.join(__dirname, 'evidence-v0.1.1');
if (record && fs.existsSync(destination)) throw new Error('committed evidence destination exists');
if (fs.existsSync(runs)) throw new Error('runs already exists; move it aside before a new measurement');
fs.mkdirSync(runs);
const matrix = [['pb861','default'], ['pb862','default'], ['pb862','preserve'],
  ['pb880','default'], ['pb880','preserve'], ['go','preserve']];
const sha = p => crypto.createHash('sha256').update(fs.readFileSync(p)).digest('hex');
const records = [];
function invoke(args,dir) {
  const p = spawnSync(cli,args,{cwd:root,encoding:'utf8',timeout:60000,maxBuffer:1<<20});
  if (p.error || p.signal) throw p.error || new Error(p.signal);
  const report = JSON.parse(fs.readFileSync(path.join(dir,'report.json')));
  // The contract is invariant across versions/options. No outcome assertion here.
  if (![0,1].includes(p.status) || !['PASS','FAIL'].includes(report.outcome) || report.incomplete
      || report.executed_cases !== report.planned_cases) throw new Error(p.stdout+p.stderr);
  const code = report.outcome === 'PASS' ? 0 : 1;
  if (code !== p.status) throw new Error('exit/report mismatch');
  return report;
}
for (const [alias,option] of matrix) {
  const version = alias==='go' ? 'v1.36.12' : require(`${alias}/package.json`).version;
  for (const shape of ['simple','nested']) {
    const name = shape==='simple' ? 'validation.Envelope' : 'validation.NestedEnvelope';
    const field = shape==='simple' ? 'future_note' : 'child.future_note';
    const argv = alias==='go' ? [goRelay,'-schema',path.join(__dirname,'old.pb'),'-message',name,'-mode','preserve']
      : [process.execPath,path.join(__dirname,'relay.cjs'),alias,option,name];
    const key = `${version}-${option}-${shape}`;
    const results = [];
    for (let i=1;i<=trials;i++) {
      const dir = path.join(runs,`${key}-${i}`);
      const config = {old_descriptor:path.join(__dirname,'old.pb'),new_descriptor:path.join(__dirname,'new.pb'),
        message:name,seeds:[path.join(__dirname,shape==='simple'?'seed.bin':'nested-seed.bin')],adapter:argv,
        working_dir:root,fields:[field],controls:shape==='simple'?['id']:['id','child.label'],
        validation_runtime:{name:alias==='go'?'google.golang.org/protobuf':'protobuf.js',version,options:option}};
      const configPath = path.join(runs,`${key}-${i}.json`);fs.writeFileSync(configPath,JSON.stringify(config,null,2)+'\n');
      const report = invoke(['check','-config',configPath,'-out',dir],dir);
      const caseDir = path.join(dir,'s001-baseline');
      const replayDir = path.join(runs,`${key}-${i}-replay`);
      const replay = invoke(['replay','-case',caseDir,'-out',replayDir],replayDir);
      if (report.cases[0].outcome !== replay.outcome
          || sha(path.join(caseDir,'input.bin')) !== sha(path.join(replayDir,'replay/input.bin')))
        throw new Error('replay outcome/input changed');
      if (alias!=='go') {
        const observed = JSON.parse(fs.readFileSync(path.join(caseDir,'stderr.log'),'utf8'));
        if (observed.version!==version || observed.option!==option) throw new Error('runtime metadata mismatch');
      }
      results.push({outcome:report.outcome,replay_outcome:replay.outcome,
        report_sha256:sha(path.join(dir,'report.json')),cases:report.cases.map(c=>({id:c.id,outcome:c.outcome,
          input_sha256:c.sha256['input.bin'],output_sha256:c.sha256['output.bin'],
          assertions:c.assertions.map(a=>({path:a.path,expected:a.expected,actual:a.actual,preserved:a.preserved}))}))});
    }
    // Successful executions must be byte-stable at the JSON-report level.
    if (new Set(results.map(r=>r.report_sha256)).size!==1) throw new Error(`unstable reports: ${key}`);
    records.push({runtime:alias==='go'?'google.golang.org/protobuf':'protobuf.js',version,option,shape,trials,
      outcome:results[0].outcome,stable:true,results});
  }
}
// Separate empty-output controls keep the original measurement matrix intact.
const emptyRecords = [];
const emptySeed = path.join(runs, 'empty-seed.bin');
const onlyNewSeed = path.join(runs, 'only-new-seed.bin');
fs.writeFileSync(emptySeed, Buffer.alloc(0));
fs.writeFileSync(onlyNewSeed, Buffer.from([0x12, 6, ...Buffer.from('future')]));
for (const [alias, option, shape, expected] of [
  ['pb862', 'default', 'only-new', 'FAIL'],
  ['pb862', 'preserve', 'only-new', 'PASS'],
  ['go', 'preserve', 'empty-seed', 'PASS']
]) {
  const version = alias === 'go' ? 'v1.36.12' : require(`${alias}/package.json`).version;
  const argv = alias === 'go' ? [goRelay, '-schema', path.join(__dirname, 'old.pb'), '-message', 'validation.Envelope', '-mode', 'preserve']
    : [process.execPath, path.join(__dirname, 'relay.cjs'), alias, option, 'validation.Envelope'];
  const key = `${version}-${option}-${shape}`;
  const results = [];
  for (let i = 1; i <= trials; i++) {
    const config = {old_descriptor:path.join(__dirname,'old.pb'), new_descriptor:path.join(__dirname,'new.pb'),
      message:'validation.Envelope', seeds:[shape === 'empty-seed' ? emptySeed : onlyNewSeed],
      adapter:argv, working_dir:root, fields:['future_note'], empty_output:'message',
      validation_runtime:{name:alias === 'go' ? 'google.golang.org/protobuf' : 'protobuf.js', version, options:option}};
    const configPath = path.join(runs, `${key}-${i}.json`);
    fs.writeFileSync(configPath, JSON.stringify(config, null, 2)+'\n');
    const dir = path.join(runs, `${key}-${i}`);
    const report = invoke(['check', '-config', configPath, '-out', dir], dir);
    const caseDir = path.join(dir, 's001-baseline');
    const baseline = report.cases[0];
    const a = baseline.assertions[0];
    if (report.outcome !== expected || baseline.outcome !== expected || a.path !== 'future_note'
        || a.expected.value.value !== (shape === 'empty-seed' ? '' : 'future')
        || a.actual.value.value !== (expected === 'FAIL' || shape === 'empty-seed' ? '' : 'future')
        || a.preserved !== (expected === 'PASS')) throw new Error(`empty-message assertion changed: ${key}`);
    if ((expected === 'FAIL' || shape === 'empty-seed') && fs.statSync(path.join(caseDir, 'output.bin')).size !== 0)
      throw new Error('control did not return an empty message');
    if (alias !== 'go') {
      const observed = JSON.parse(fs.readFileSync(path.join(caseDir,'stderr.log'),'utf8'));
      if (observed.version !== version || observed.option !== option) throw new Error('runtime metadata mismatch');
    }
    const replayDir = path.join(runs, `${key}-${i}-replay`);
    const replay = invoke(['replay', '-case', caseDir, '-out', replayDir], replayDir);
    const saved = JSON.parse(fs.readFileSync(path.join(replayDir,'replay/case.json')));
    if (replay.outcome !== expected || saved.config.empty_output !== 'message'
        || sha(path.join(caseDir,'input.bin')) !== sha(path.join(replayDir,'replay/input.bin'))
        || JSON.stringify(baseline.assertions) !== JSON.stringify(replay.cases[0].assertions))
      throw new Error('empty-message replay changed contract/input/assertions');
    results.push({outcome:report.outcome, replay_outcome:replay.outcome, report_sha256:sha(path.join(dir,'report.json')),
      cases:report.cases.map(c=>({id:c.id,outcome:c.outcome,input_sha256:c.sha256['input.bin'],output_sha256:c.sha256['output.bin'],assertions:c.assertions}))});
  }
  if (new Set(results.map(r=>r.report_sha256)).size !== 1) throw new Error(`unstable reports: ${key}`);
  emptyRecords.push({runtime:alias === 'go' ? 'google.golang.org/protobuf' : 'protobuf.js', version, option, shape,
    empty_output:'message', trials, outcome:expected, stable:true, results});
}
// Replay actual committed v0.1.0 bundles with portable adapter overrides.
const legacyReplays = [];
for (const key of ['8.6.2-default-simple','8.6.2-default-nested','8.6.2-preserve-nested','v1.36.12-preserve-simple']) {
  const nested = key.endsWith('nested');
  const go = key.startsWith('v');
  const option = key.includes('-default-') ? 'default' : 'preserve';
  const name = nested ? 'validation.NestedEnvelope' : 'validation.Envelope';
  const argv = go ? [goRelay,'-schema',path.join(__dirname,'old.pb'),'-message',name,'-mode','preserve']
    : [process.execPath,path.join(__dirname,'relay.cjs'),'pb862',option,name];
  const bundle = path.join(__dirname,'evidence',key);
  for (const id of fs.readdirSync(bundle).filter(n => n.startsWith('s001-'))) {
    const source = path.join(bundle,id);
    const original = JSON.parse(fs.readFileSync(path.join(source,'case.json')));
    if (original.format_version !== 1 || original.engine.tool !== '0.1.0') throw new Error('legacy fixture replaced');
    const dir = path.join(runs, `legacy-${key}-${id}`);
    const report = invoke(['replay','-case',source,'-out',dir,'-working-dir',root,'-adapter',JSON.stringify(argv)],dir);
    const saved = JSON.parse(fs.readFileSync(path.join(dir,'replay/case.json')));
    if (report.outcome !== original.case.outcome || saved.config.empty_output !== 'reject'
        || saved.replay_source_engine.tool !== '0.1.0'
        || sha(path.join(source,'input.bin')) !== sha(path.join(dir,'replay/input.bin'))
        || JSON.stringify(original.case.assertions) !== JSON.stringify(report.cases[0].assertions))
      throw new Error('legacy replay changed outcome/input/assertions');
    legacyReplays.push({bundle:key,case:id,producer:original.engine.tool,outcome:report.outcome,input_sha256:saved.case.sha256['input.bin']});
  }
}
const summary = {format_version:1,node:process.version,tool:JSON.parse(fs.readFileSync(path.join(runs,'8.6.1-default-simple-1/report.json'))).engine,
  package_lock_sha256:sha(path.join(__dirname,'package-lock.json')),old_descriptor_sha256:sha(path.join(__dirname,'old.pb')),
  new_descriptor_sha256:sha(path.join(__dirname,'new.pb')),records,empty_records:emptyRecords,legacy_replays:legacyReplays};
fs.writeFileSync(path.join(runs,'summary.json'),JSON.stringify(summary,null,2)+'\n');
for (const r of records) console.log(`${r.runtime} ${r.version} ${r.option} ${r.shape}: ${r.outcome} (${trials} checks + ${trials} exact-input replays, stable)`);
for (const r of emptyRecords) console.log(`empty_output=message ${r.runtime} ${r.version} ${r.option} ${r.shape}: ${r.outcome} (${trials} checks + ${trials} replays, stable)`);
console.log(`Legacy v0.1.0 exact-input replays: ${legacyReplays.length}`);
console.log(`Evidence: ${runs}`);
if (record) {
  // Save representative cases alongside the current release's measurements.
  fs.mkdirSync(destination);
  for (const key of ['8.6.2-default-simple','8.6.2-default-nested','8.6.2-preserve-nested','v1.36.12-preserve-simple'])
    fs.cpSync(path.join(runs,`${key}-1`),path.join(destination,key),{recursive:true});
  for (const key of ['8.6.2-default-only-new','8.6.2-preserve-only-new','v1.36.12-preserve-empty-seed'])
    fs.cpSync(path.join(runs,`${key}-1`),path.join(destination,key),{recursive:true});
  fs.copyFileSync(path.join(runs,'summary.json'),path.join(__dirname,'results-v0.1.1.json'));
}
