/** Scry's source-bound Go gate. No deployment or private runtime authority. */
import { argument, dag, type Directory, func, object, type Platform } from '@dagger.io/dagger';

const GO_IMAGE = 'golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b';
const NODE_IMAGE = 'node:22.22.0-bookworm-slim';
const PLAYWRIGHT_VERSION = '1.63.0';
const GITLEAKS_IMAGE = 'zricethezav/gitleaks:v8.30.1@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f';

@object()
export class Scry {
  /**
   * Check and export one binary, never rebuild after smoke. Source is the frozen
   * source/ + source.json package made by `python3 scripts/scry-ci full`, not an
   * arbitrary working tree with a separately supplied Git label.
   */
  @func()
  async check(
    @argument({ ignore: ['.git', '**/.git'] }) source: Directory,
  ): Promise<Directory> {
    const scan = dag.container()
      .from(GITLEAKS_IMAGE)
      .withDirectory('/src', source.directory('source'))
      .withWorkdir('/src')
      .withExec(['gitleaks', 'dir', '/src', '--redact', '--no-banner']);
    const secretsProof = (await scan.stdout()) + (await scan.stderr());
    // Browser runtime for the critic suite: pinned Playwright module (no
    // bundled browser download) copied into the gate, plus the distribution's
    // Chromium. The critic tests require a real browser in this gate.
    const criticRuntime = dag.container({ platform: 'linux/amd64' as Platform })
      .from(NODE_IMAGE)
      .withEnvVariable('PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD', '1')
      .withExec(['mkdir', '-p', '/opt/scry-critics'])
      .withWorkdir('/opt/scry-critics')
      .withExec(['npm', 'install', '--no-audit', '--no-fund', '--loglevel=error', `playwright@${PLAYWRIGHT_VERSION}`]);
    // Resolve Worker/runtime imports from the committed lockfile. Keep this
    // outside /input/source so the frozen source inventory remains unchanged.
    const workerDependencies = dag.container({ platform: 'linux/amd64' as Platform })
      .from(NODE_IMAGE)
      .withDirectory('/input', source)
      .withWorkdir('/input/source')
      .withExec(['npm', 'ci', '--ignore-scripts', '--no-audit', '--no-fund', '--loglevel=error']);
    const node = dag.container({ platform: 'linux/amd64' as Platform }).from(NODE_IMAGE);
    return dag.container({ platform: 'linux/amd64' as Platform })
      .from(GO_IMAGE)
      .withExec(['apt-get', 'update'])
      .withExec(['apt-get', 'install', '-y', '--no-install-recommends', 'python3', 'chromium'])
      .withFile('/usr/local/bin/node', node.file('/usr/local/bin/node'))
      .withDirectory('/opt/scry-critics', criticRuntime.directory('/opt/scry-critics'))
      .withMountedCache('/go/pkg/mod', dag.cacheVolume('scry-go-mod-1.27'))
      .withMountedCache('/root/.cache/go-build', dag.cacheVolume('scry-go-build-1.27'))
      .withDirectory('/input', source)
      .withDirectory('/input/node_modules', workerDependencies.directory('/input/source/node_modules'))
      .withNewFile('/input/gitleaks.txt', secretsProof)
      .withWorkdir('/input/source')
      .withExec([
        'python3', 'scripts/scry-ci', 'gate',
        '--source', '/input/source', '--metadata', '/input/source.json',
        '--secrets-proof', '/input/gitleaks.txt', '--out', '/out',
      ])
      .directory('/out');
  }
}
