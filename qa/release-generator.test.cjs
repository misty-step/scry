// Real release-please Rust strategy and updaters over isolated public source.
// Only its file transport is local; no GitHub write, model or deployment occurs.
const assert = require('node:assert/strict');
const {readFileSync} = require('node:fs');
const {createRequire} = require('node:module');
const {resolve, dirname} = require('node:path');
const test = require('node:test');
const tool = createRequire(resolve(process.env.RELEASE_PLEASE_MODULE, 'package.json'));
assert.equal(tool('./package.json').version, '17.6.0');
tool('./build/src/index.js'); // Initialize the library's strategy registry first.
const base = dirname(tool.resolve('./build/src/index.js'));
const {Rust} = require(resolve(base, 'strategies/rust.js'));
const {Version} = require(resolve(base, 'version.js'));
const {TagName} = require(resolve(base, 'util/tag-name.js'));
const {parseConventionalCommits} = require(resolve(base, 'commit.js'));
const config = JSON.parse(readFileSync('release-please-config.json'));
const manifest = JSON.parse(readFileSync('.release-please-manifest.json'));
const options = config.packages['.'];
const github = {
  repository: {owner: 'misty-step', repo: 'scry', defaultBranch: 'master'},
  async getFileContentsOnBranch(path) {
    const parsedContent = readFileSync(path, 'utf8');
    return {parsedContent, content: Buffer.from(parsedContent).toString('base64'), sha: 'local-source'};
  }
};
const latest = {tag: new TagName(Version.parse('0.1.0')), sha: config['bootstrap-sha'], notes: ''};
async function generate(message) {
  const strategy = new Rust({github, targetBranch: 'master', includeComponentInTag: options['include-component-in-tag'], extraFiles: options['extra-files']});
  const commits = parseConventionalCommits([{sha: 'a'.repeat(40), message, files: ['src/web.rs']}]);
  const pr = await strategy.buildReleasePullRequest(commits, latest);
  if (!pr) return undefined;
  const files = Object.fromEntries(pr.updates.map(update => [update.path, update.updater.updateContent(readFileSync(update.path, 'utf8'))]));
  return {version: pr.version.toString(), body: pr.body.toString(), files};
}
for (const [message, expected] of [
  ['fix: Keep saved answers after reconnecting', '0.1.1'],
  ['feat: Read the changes included in your notebook', '0.2.0'],
  ['feat!: Replace the export format\n\nBREAKING CHANGE: Export before upgrading; keep the previous compatible build to roll back.', '1.0.0']
]) {
  test(`${message.split('\n')[0]} -> ${expected}, useful notes and every manifest`, async () => {
    const first = await generate(message);
    assert.equal(first.version, expected);
    assert.match(first.files['Cargo.toml'], new RegExp('version = "' + expected.replaceAll('.', '\\.') + '"'));
    assert.ok(first.files['Cargo.lock'].includes('name = "scry"\nversion = "' + expected + '"'));
    assert.equal(JSON.parse(first.files['package.json']).version, expected);
    const npm = JSON.parse(first.files['package-lock.json']);
    assert.equal(npm.version, expected);
    assert.equal(npm.packages[''].version, expected);
    assert.ok(first.files['CHANGELOG.md'].includes(expected));
    assert.ok(first.files['CHANGELOG.md'].includes(message.split('\n')[0].replace(/^\w+!?: /, '')));
    assert.deepEqual(await generate(message), first, 'Identical inputs must produce identical output');
    if (expected === '1.0.0') assert.match(first.files['CHANGELOG.md'], /Export before upgrading/);
  });
}
test('non-user changes and empty input do not cut a release', async () => {
  for (const type of ['chore', 'docs', 'test', 'build', 'ci', 'refactor', 'style']) assert.equal(await generate(type + ': Maintain tooling'), undefined);
  const strategy = new Rust({github, targetBranch: 'master'});
  assert.equal(await strategy.buildReleasePullRequest([], latest), undefined);
});
test('the unreleased Rust unit starts at 0.1.0 without a fictional compare tag', async () => {
  const strategy = new Rust({github, targetBranch: 'master', includeComponentInTag: false});
  const pr = await strategy.buildReleasePullRequest(parseConventionalCommits([{sha:'b'.repeat(40),message:'feat: Read the changes included in your notebook'}]));
  assert.equal(pr.version.toString(), '0.1.0');
  assert.ok(!pr.body.toString().includes('compare/v0.0.0'));
});
test('baseline is explicit, not a guessed historical release', () => {
  assert.match(config['bootstrap-sha'], /^[a-f0-9]{40}$/);
  assert.ok(Version.parse(manifest['.']));
  assert.equal(options['release-type'], 'rust');
  assert.equal(options['bump-minor-pre-major'], false);
  assert.equal(options['bump-patch-for-minor-pre-major'], false);
});
