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
const record = process.argv.includes('--record');
const destination = path.join(__dirname, 'evidence');
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
const summary = {format_version:1,node:process.version,tool:JSON.parse(fs.readFileSync(path.join(runs,'8.6.1-default-simple-1/report.json'))).engine,
  package_lock_sha256:sha(path.join(__dirname,'package-lock.json')),old_descriptor_sha256:sha(path.join(__dirname,'old.pb')),
  new_descriptor_sha256:sha(path.join(__dirname,'new.pb')),records};
fs.writeFileSync(path.join(runs,'summary.json'),JSON.stringify(summary,null,2)+'\n');
for (const r of records) console.log(`${r.runtime} ${r.version} ${r.option} ${r.shape}: ${r.outcome} (${trials} checks + ${trials} exact-input replays, stable)`);
console.log(`Evidence: ${runs}`);
if (record) {
  // Keep compact, directly replayable negative and positive controls for review.
  fs.mkdirSync(destination);
  for (const key of ['8.6.2-default-simple','8.6.2-default-nested','8.6.2-preserve-nested','v1.36.12-preserve-simple'])
    fs.cpSync(path.join(runs,`${key}-1`),path.join(destination,key),{recursive:true});
  fs.copyFileSync(path.join(runs,'summary.json'),path.join(__dirname,'results.json'));
}
