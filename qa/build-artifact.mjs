#!/usr/bin/env node
import {execFileSync} from 'node:child_process';
import {createHash} from 'node:crypto';
import {mkdirSync, readFileSync, writeFileSync} from 'node:fs';
import {resolve, join} from 'node:path';

const [candidate, destination] = process.argv.slice(2);
if (!/^[a-f0-9]{40}$/.test(candidate || '') || !destination) throw Error('usage: node qa/build-artifact.mjs <exact-sha> <new-output-dir>');
const git = (...args) => execFileSync('git', args, {encoding: 'utf8'}).trim();
const tree = git('rev-parse', `${candidate}^{tree}`);
const output = resolve(destination);
mkdirSync(output); // Exclusive creation prevents replacing previously verified bytes.
const source = join(output, 'source');
mkdirSync(source);
execFileSync('tar', ['-x', '-C', source], {input: execFileSync('git', ['archive', candidate], {maxBuffer: 32 << 20})});
const cache = resolve('target/qa-cache');
const env = {PATH: process.env.PATH, HOME: output, LANG: 'C.UTF-8', GOPATH: join(cache, 'go-path'), GOMODCACHE: join(cache, 'go-mod'), GOCACHE: join(cache, 'go-build'), CGO_ENABLED: '0'};
const run = args => execFileSync('go', args, {cwd: source, env, encoding: 'utf8', maxBuffer: 16 << 20});
const manifest = {schema: 'scry-qa-artifact/1', candidate, tree, built_at: new Date().toISOString(), toolchain: run(['version']).trim(), files: [], commands: []};
function build(path, args, cwd = source) {
  manifest.commands.push(['go', ...args]);
  execFileSync('go', args, {cwd, env, stdio: ['ignore', 'pipe', 'pipe']});
  manifest.files.push({path, sha256: createHash('sha256').update(readFileSync(join(output, path))).digest('hex')});
}
build('scry', ['build', '-mod=readonly', '-trimpath', '-ldflags', `-X main.revision=${candidate}`, '-o', join(output, 'scry'), './cmd/scry']);
for (const pkg of ['learning', 'store', 'semantic', 'web']) build(`${pkg}.test`, ['test', '-c', '-mod=readonly', '-o', join(output, `${pkg}.test`), `./internal/${pkg}`]);
// This independently authored probe calls the candidate's pure module API.
// It is a separate verifier artifact and does not modify the Scry executable.
const probeDir = join(source, 'qa-boundary-probe');
mkdirSync(probeDir);
writeFileSync(join(probeDir, 'main.go'), readFileSync(new URL('./boundary-probe.go.txt', import.meta.url)));
build('boundary-probe', ['build', '-mod=readonly', '-trimpath', '-o', join(output, 'boundary-probe'), './qa-boundary-probe']);
manifest.probe_source_sha256 = createHash('sha256').update(readFileSync(join(probeDir, 'main.go'))).digest('hex');
writeFileSync(join(output, 'artifact.json'), JSON.stringify(manifest, null, 2) + '\n');
console.log(join(output, 'artifact.json'));
