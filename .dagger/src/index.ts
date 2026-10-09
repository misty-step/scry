/** Scry's frozen-source Rust/WASM gate. No deployment or production authority. */
import { argument, dag, type Directory, func, object, type Platform } from '@dagger.io/dagger';

const RUST_IMAGE = 'rust:1.98.1-bookworm@sha256:c49256cbe5ea0188bc658a689500d70c41eb51f009a7a7be209caf60a944f3ec';
const NODE_IMAGE = 'node:22.22.0-bookworm-slim';
// Only the archived US-001.1 migration test uses Go; the active application is Rust.
const RETAINED_GO_IMAGE = 'golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b';
const GITLEAKS_IMAGE = 'zricethezav/gitleaks:v8.30.1@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f';

@object()
export class Scry {
  /** Export the exact smoke-tested Worker/WASM bundle and source proof. */
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
    const dependencies = dag.container({ platform: 'linux/amd64' as Platform })
      .from(NODE_IMAGE)
      .withDirectory('/input', source)
      .withEnvVariable('PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD', '1')
      .withWorkdir('/input/source')
      .withExec(['npm', 'ci', '--ignore-scripts', '--no-audit', '--no-fund', '--loglevel=error']);
    const node = dag.container({ platform: 'linux/amd64' as Platform }).from(NODE_IMAGE);
    const retainedGo = dag.container({ platform: 'linux/amd64' as Platform }).from(RETAINED_GO_IMAGE);
    return dag.container({ platform: 'linux/amd64' as Platform })
      .from(RUST_IMAGE)
      .withExec(['apt-get', 'update'])
      .withExec(['apt-get', 'install', '-y', '--no-install-recommends', 'python3', 'chromium', 'openssl', 'libssl-dev', 'pkg-config'])
      .withExec(['rustup', 'component', 'add', 'rustfmt', 'clippy'])
      .withExec(['rustup', 'target', 'add', 'wasm32-unknown-unknown'])
      .withExec(['cargo', 'install', 'worker-build', '--version', '0.8.7', '--locked'])
      .withFile('/usr/local/bin/node', node.file('/usr/local/bin/node'))
      .withDirectory('/usr/local/go', retainedGo.directory('/usr/local/go'))
      .withEnvVariable('PATH', '/usr/local/go/bin:/usr/local/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin')
      .withMountedCache('/usr/local/cargo/registry', dag.cacheVolume('scry-rust-registry-1.98'))
      .withDirectory('/input', source)
      .withDirectory('/input/node_modules', dependencies.directory('/input/source/node_modules'))
      .withEnvVariable('SCRY_CRITICS_CHROMIUM_PATH', '/usr/bin/chromium')
      .withEnvVariable('WRANGLER_SEND_METRICS', 'false')
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
