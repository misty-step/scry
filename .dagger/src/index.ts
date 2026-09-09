/** Scry's source-bound Go gate. No deployment or private runtime authority. */
import { argument, dag, type Directory, func, object, type Platform } from '@dagger.io/dagger';

const GO_IMAGE = 'golang:1.27.1-bookworm@sha256:648f440f42a0958804efb24df176f806f9d353b41f1c0627f666428e40310f6b';
const NODE_IMAGE = 'node:22.22.0-bookworm-slim';
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
    const node = dag.container({ platform: 'linux/amd64' as Platform }).from(NODE_IMAGE);
    return dag.container({ platform: 'linux/amd64' as Platform })
      .from(GO_IMAGE)
      .withExec(['apt-get', 'update'])
      .withExec(['apt-get', 'install', '-y', '--no-install-recommends', 'python3'])
      .withFile('/usr/local/bin/node', node.file('/usr/local/bin/node'))
      .withMountedCache('/go/pkg/mod', dag.cacheVolume('scry-go-mod-1.27'))
      .withMountedCache('/root/.cache/go-build', dag.cacheVolume('scry-go-build-1.27'))
      .withDirectory('/input', source)
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
