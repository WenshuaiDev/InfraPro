// Render the published ARC table from exactly the same assertions as the CLI.
import { existsSync, writeFileSync } from 'node:fs';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { read, root, runDirectory } from './architecture-lab.mjs';
import { sharedConsoleFor } from './architecture-shared-console.mjs';
import { visualOutputDirectory } from './architecture-visual-frontend.mjs';

const status = { passed: '通过', failed: '未通过', incomplete: '待补证据', skipped: '按用户要求跳过' };
const inline = (text) => '\u0060' + text + '\u0060';
const cell = (value) => String(value).replaceAll('|', '\\|').replaceAll('\n', ' ');
const number = (value) => Number(value).toLocaleString('zh-CN');

function suite(id) {
  const n = Number(id.slice(-3));
  if (n === 1) return 'routes';
  if (n <= 15) return 'parity';
  if (n <= 17) return 'visual';
  if (n <= 31 || n === 36) return 'faults';
  if (n <= 35) return 'restore';
  if (n <= 38) return 'load';
  return '全部';
}

export function renderVerification(dir, ledger, destination = join(dir, 'verification.md')) {
  const get = (file) => (existsSync(join(dir, file)) ? read(join(dir, file)) : null);
  const closeout = get('scope/user-closeout.json');
  const line = (label, absolute) =>
    '[' + label + '](' + relative(dirname(destination), absolute) + ')';
  const evidence = (file) => line(inline(file), join(dir, file));
  const sharedConsole = sharedConsoleFor(dir);
  const visualDir = visualOutputDirectory(dir, sharedConsole);
  const visualPath = file => relative(dir, join(visualDir, file));
  const reviewMatrices = ['b', 'self']
    .map(side => visualPath('visual/paired/' + side + '/matrix.json'))
    .filter(path => existsSync(join(dir, path)));
  const report = (fields) => fields.filter(Boolean).map(cell).join(' · ');
  const b = get('baseline.json'),
    selected = get('candidate.evaluated.json'),
    coverage = get('http/coverage.json'),
    deploy = get('deploy/deployment.json');
  const worker = get('worker-checks/summary.json'),
    events = get('events/checks.json');
  const offline = get('deploy/offline-release.json');
  const originalDiagnostics = get('regression/baseline/test-diagnostics-unmodified.json');
  const membership = get('regression/membership-planners/qualification.json');
  const pagination = get('regression/baseline/frozen-diagnostics-browser/worker-pagination.json');
  const pairedDiagnostics = get('diagnostics-comparison.json');
  const currentDiagnostics = pairedDiagnostics?.baseline?.source === b?.source_archive_sha256 &&
    pairedDiagnostics?.candidate?.source === selected?.inputs?.sourceSHA256 && pairedDiagnostics?.common_frame_clock?.passed;
  const archivedSource =
    selected && existsSync(join(selected.directory, 'application-source.json'))
      ? read(join(selected.directory, 'application-source.json'))
      : null;
  const ingestion = get('ingestion/comparison.json'),
    display = get('load/display/comparison.json');
  const responseProof = get('worker-checks/response-staging-fix/verified.json');
  const storageRace = get('worker-checks/monitoring-storage-race/verified.json');
  const consoleCorrections = get('console-corrections/release.json');
  const originalBrowserPath = 'regression/browser-diagnoses/frozen-s0-9a50daedc25e12d7/manifest.json';
  const originalBrowser = get(originalBrowserPath);
  const firstSharedBrowserPath = 'regression/browser-diagnoses/c1-original-295-failures.json';
  const firstSharedBrowser = get(firstSharedBrowserPath);
  const activityPath = 'worker-checks/identity-b9-final-originals/clock-handoff.json';
  const activity = get(activityPath);
  const sourceFocusPath = 'regression/browser-diagnoses/monitoring-import-escape-focus.json';
  const sourceFocus = get(sourceFocusPath);
  const focusPath = 'console-corrections/import-focus.json';
  const correctedFocus = get(focusPath);
  const retainedFocusPath = 'manual/live-import-focus.json';
  const retainedFocus = get(retainedFocusPath);
  const currentRetainedFocus = retainedFocus?.passed && retainedFocus.consoleSHA256 === sharedConsole?.console.sha256 &&
    ['a','b'].every(side => {
      const run = retainedFocus.sides?.[side], walk = get('manual/walkthrough-' + side + '.json');
      return run?.passed && run.launchID && run.launchID === walk?.execution?.launchID &&
        run.backendSHA256 === (side === 'a' ? b?.source_archive_sha256 : selected?.inputs?.sourceSHA256);
    });
  const activityResults = ['baseline','candidate'].map(side => 'identity/session-activity/' + side + '/result.json');
  const originalStorage = get('manual/original-storage-oom-recovery.json');
  const browserCausesPath = 'regression/browser-diagnoses/c1-second-shard-causes.json';
  const browserCauses = get(browserCausesPath);
  const frozenDisplayError = get('visual/frozen-display-detail-exception.json');
  const frozenLocks = get('visual/frozen-accounts-locks.json');
  const observedDisplay = get('visual/frozen-display-detail-recheck.json');
  const tenantAudit = get('visual/recording-probes/tenant-audit-history.json');
  const visualFindings = [];
  if (frozenDisplayError?.errors?.some(error => /null.*forEach/.test(error))) {
    visualFindings.push(
      '冻结版通过公开 API 为已发布大屏设置仅含资源的授权后，网页可读到实例详情，打开管理抽屉时却在 `bindingSummary` 处理空值时抛出异常。授权编辑器会提交资源、通道、空间资料、分析成果四个列表。两种输入及其公开响应应分别核对。' +
        evidence('visual/frozen-display-detail-exception.json') + ' · ' +
        evidence('visual/frozen-display-config-shape.json'),
    );
    if (observedDisplay?.passed)
      visualFindings.push('用原编辑器的完整列表结构保存相同资源和期限后，已在冻结版真实页面看到已发布实例及授权摘要。' +
        evidence('visual/frozen-display-detail-recheck.json'));
  }
  if (frozenLocks?.commonCycles > 0) {
    visualFindings.push('冻结版派生和空间页面的额外状态曾收到 503。同批合成数据库记录了 ' +
      frozenLocks.commonCycles + ' 次两个事务都在账户行等待 `FOR UPDATE` 的循环。请求与数据库记录各自保留其时点。' +
      evidence('visual/frozen-authorized-network.json') + ' · ' + evidence('visual/frozen-accounts-locks.json'));
  }
  if (browserCauses?.sourceSHA256 === b?.source_archive_sha256 &&
      browserCauses?.consoleSHA256 === sharedConsole?.console?.sha256 &&
      browserCauses.storage.deadlocks?.length === 3) {
    visualFindings.push('冻结后端使用修正控制台重跑时，派生页的最新值和两次趋势请求曾返回 503。三个请求分别对应合成 PostgreSQL 中同一账户行上的一次死锁。同一窗口，该测试数据库在 ClickHouse 上执行的 10 次查询均成功。账号概览和工程绑定的另两次超时发生在登录阶段。' +
      evidence(browserCausesPath));
  }
  if (tenantAudit?.candidate === selected?.inputs?.sourceSHA256 &&
      tenantAudit?.first?.status?.a === 200 && tenantAudit.first.status.b === 503 &&
      tenantAudit.actualBrowserPagination?.sameEventsAndOrder) {
    visualFindings.push('一次租户审计列表访问在冻结版返回 200，重构版返回 503。使用相同权限和独立恢复的数据，两侧首读均返回相同的 10 条记录。随后用各自服务器签发的游标，在真实浏览器中翻到第二页再返回，两版的事件和顺序一致。记录保留了最初的失败、数据库观测和页面截图。' +
      evidence('visual/recording-probes/tenant-audit-history.json'));
  }
  const performance = [];
  if (
    selected &&
    ingestion?.candidate_release?.source_sha256 === selected.inputs.sourceSHA256 &&
    ingestion.rounds?.length === 9
  ) {
    const rows = Object.entries({
      a: 'A 原拓扑',
      pg: 'B 按 PG 扫描',
      mq: 'B 开启 MQ',
    }).map(([key, name]) => {
      const m = ingestion.medians[key];
      return (
        '| ' +
        [
          name,
          (m.end_to_end_p95_ms / 1000).toFixed(2),
          m.catchup_seconds.toFixed(2),
          m.completion_fps.toFixed(2),
          (m.ready_to_first_claim_p95_ms / 1000).toFixed(3),
          m.scans_per_1000_jobs?.toFixed(2) ?? '不适用',
        ].join(' | ') +
        ' |'
      );
    });
    performance.push(
      '### 接入的三轮中位数',
      '| 拓扑 | 云端收到至有权读回 P95（秒） | 恢复至零待办（秒） | 完成速率（帧/秒） | 耐久就绪至首次领取 P95（秒） | 每千任务 PG 候选扫描数 |\n| --- | ---: | ---: | ---: | ---: | ---: |\n' +
        rows.join('\n'),
      'B 开启 MQ 相对 B 的 PG 保底路径，首次领取 P95 比值为 ' +
        ingestion.net.claim_p95_ratio.toFixed(3) +
        '，每千任务的 PG 候选扫描数比值为 ' +
        ingestion.net.scan_ratio.toFixed(3) +
        '。每轮的输入、终态、分类摘要、候选扫描、锁等待、物理连接、CPU 和内存样本见 ' +
        evidence('ingestion/campaigns/' + ingestion.campaign + '/comparison.json') +
        '。',
    );
  }
  const displayProgress = get('load/display/progress.json');
  if (
    selected &&
    display?.provenance?.candidate === selected.inputs.sourceSHA256 &&
    display.campaign === displayProgress?.campaign &&
    display.rounds?.length === 18 &&
    display.relative?.length === 22
  ) {
    const labels = {
      hot: '热统计',
      directory: '目录',
      cold: '冷统计',
      login: '登录',
      ledger: '台账',
      curve: '控制台曲线',
      stream: '大文件',
      publish: '正式发布',
    };
    const rows = Object.entries(labels).map(([metric, label]) => {
      const values = ['idle', 'history', 'mixed'].map((scene) => {
        const m = display.relative.find((item) => item.metric === metric && item.scene === scene);
        if (!m) return '—';
        const text =
          (m.baselineMedianP95Ms / 1000).toFixed(3) +
          ' → ' +
          (m.candidateMedianP95Ms / 1000).toFixed(3) +
          '（' +
          m.ratio.toFixed(3) +
          '）';
        return m.passed ? text : '**' + text + '，超限**';
      });
      return '| ' + [label, ...values].join(' | ') + ' |';
    });
    const folder = 'load/display/campaigns/' + display.campaign + '/';
    const profile = get(folder + 'resource-profile.json');
    const sample = get(folder + 'a-1-idle.json');
    const budget = sample?.budget?.total;
    if (budget?.length === 4) {
      performance.push(
        '### 本次大屏计量的资源分配',
        '两侧服务各分配 ' +
          budget[0] +
          ' 个 PostgreSQL 连接、' +
          budget[1] +
          ' 个 ClickHouse 连接、' +
          budget[2] +
          ' 个 Go 调度线程和 ' +
          number(budget[3]) +
          ' MiB Go 运行时内存目标，对象请求同时最多 ' +
          sample.budget.objectRequests.limit +
          ' 个。' +
          (profile
            ? '每场收据记录每个 PID 的程序摘要、实际调度与内存设置，以及观测到的数据库连接。'
            : '') +
          evidence(folder + (profile ? 'resource-profile.json' : 'a-1-idle.json')),
      );
    }
    performance.push(
      '### 大屏与同时运行的业务',
      '表内给出三个独立轮次的 P95 中位数，单位为秒。箭头由冻结版 A 指向新版 B，括号内为 B/A。相对门槛为 1.100。' +
        '热统计、目录及每次冷读另按各场的绝对门槛核验。' +
        evidence('load/display/campaigns/' + display.campaign + '/comparison.json'),
      '| 业务 | 空闲 | 历史满载 | 同时发布与冷读 |\n| --- | ---: | ---: | ---: |\n' +
        rows.join('\n'),
    );
  }
  const reg = (side) => ledger.commands.filter((c) => c.side === side && c.command?.[0] === 'pnpm');
  const env = (side) => get('environment-' + side + '.json');
  const examples = (arc, limit = 2) => {
    const checks = arc.checks.find((c) => c.part === 'paired-business-assertions')?.actual || [];
    return [...new Set(checks.filter((c) => c.passed).map((c) => c.name))].slice(0, limit);
  };
  function actual(arc) {
    const n = Number(arc.id.slice(-3)),
      checks = arc.checks;
    const required = checks.filter(c => c.status !== 'skipped');
    const summary = required.filter(c => c.passed).length + '／' + required.length + ' 项功能检查通过';
    const sw = worker?.criteria?.[arc.id]?.checks;
    if (n === 1 && coverage) {
      const detail = checks.find((c) => c.path === 'http/coverage.json').actual;
      return report([
        summary,
        number(coverage.positive) + '／' + number(coverage.total) + ' 个合法操作',
        '未取得原有拒绝结果 ' + detail.missingExpectedError.length + ' 项',
        '响应差异 ' + (detail.differences ?? '待测') + ' 项',
      ]);
    }
    if (n === 2) {
      const edges = get('http/workflows/edges.json')?.cases?.[0]?.checks;
      return report([
        summary,
        edges &&
          'Nginx 与开发代理 ' + edges.filter((r) => r.passed).length + '／' + edges.length + ' 对',
      ]);
    }
    if (n === 16) {
      const matrices = ['self', 'b'].map((s) => get(visualPath('visual/paired/' + s + '/matrix.json')));
      return report([
        summary,
        ...matrices.map(
          (m, i) =>
            (i ? 'A/B ' : 'A/A ') +
            (m ? m.rows.filter((r) => r.passed).length + '／' + m.expectedRows : '待测'),
        ),
      ]);
    }
    if (n === 17) {
      const cases = get(visualPath('http/workflows/navigation.json'))?.cases?.[0]?.checks;
      return report([
        summary,
        cases && cases.filter((r) => r.passed).length + '／' + cases.length + ' 组导航流程',
      ]);
    }
    if (sw)
      return report([
        summary,
        sw.filter((c) => c.passed).length + '／' + sw.length + ' 条 worker 回放',
        sw[0]?.name,
      ]);
    if ([23, 24, 25, 36].includes(n))
      return report([
        summary,
        events &&
          '真实消息与数据库总套件 ' +
            events.cases.filter((c) => c.state === 'pass').length +
            '／' +
            events.cases.length +
            ' 例',
      ]);
    if (n === 34) {
      const joint = get('deploy/joint-recovery.json');
      return report([
        summary,
        joint?.object_counts &&
          '私有对象清单 ' +
            Object.entries(joint.object_counts)
              .map(([name, count]) => name + ' ' + count)
              .join('、'),
        joint?.before_open && joint.before_open + ' → ' + joint.after_open,
      ]);
    }
    if (n === 35) {
      const phases = get('deploy/rollback-readback.json')?.phases ?? [];
      return report([
        summary,
        ...phases.map((p) => p.phase + ' ' + (p.readbacks?.length ?? 0) + ' 笔写入回读'),
      ]);
    }
    if ([32, 33].includes(n))
      return report([
        summary,
        ...(deploy?.checks || []).filter((c) => c.arc === arc.id).map((c) => c.key),
      ]);
    if (n === 37) {
      if (ingestion && ingestion.candidate_release?.source_sha256 !== selected?.inputs.sourceSHA256)
        return report([
          summary,
          '本版三种模式、九轮正式读回待完成',
          '已归档的上版轮次 ' + ingestion.rounds.length,
        ]);
      const modes = ['a', 'pg', 'mq'];
      const metric = (key, scale) =>
        modes.map((mode) => (ingestion.medians[mode][key] / scale).toFixed(2)).join('／');
      return report([
        summary,
        ingestion ? '已记录 ' + ingestion.rounds.length + ' 轮' : '三模式全测报告待生成',
        ingestion?.medians?.mq &&
          'A／PG／MQ 端到端 P95 中位数 ' +
            metric('end_to_end_p95_ms', 1000) +
            ' 秒，追平 ' +
            metric('catchup_seconds', 1) +
            ' 秒',
      ]);
    }
    if (n === 38) {
      const progress = get('load/display/progress.json');
      if (
        display &&
        selected &&
        display.provenance?.candidate === selected.inputs.sourceSHA256 &&
        (!progress || progress.campaign === display.campaign)
      )
        return report([
          summary,
          display.rounds.filter((r) => r.passed).length + '／18 个场景轮次通过绝对门槛及读回',
          (display.relative ?? []).filter((r) => r.passed).length + '／22 项 P95 相对门槛通过',
        ]);
      const completed =
        progress && selected && progress.candidate === selected.inputs.sourceSHA256
          ? progress.completed
          : [];
      return report([
        summary,
        '本版已保存 ' + completed.length + '／18 个首次计时场景',
        completed.length &&
          '绝对门槛通过 ' + completed.filter((item) => item.passed).length + ' 场',
      ]);
    }
    if (n === 39)
      return report([
        summary,
        ...['baseline', 'candidate'].map(
          (s, i) =>
            (i ? 'B ' : 'A ') +
            reg(s).filter((c) => c.status === 'passed').length +
            '／' +
            reg(s).filter(c => c.status !== 'skipped').length +
            ' 条功能回归命令',
        ),
      ]);
    return report([summary, ...examples(arc)]);
  }
  function links(arc) {
    const found = new Set();
    for (const c of arc.checks) {
      if (c.part === 'paired-business-assertions') {
        for (const r of c.actual) if (r.path) found.add(r.path);
      } else if (c.path) {
        if (/^regression\/(baseline|candidate)\//.test(c.path) && arc.id === 'ARC-039') {
          found.add(c.path.split('/').slice(0, 2).join('/') + '/');
        } else found.add(c.path);
      }
    }
    if (arc.id === 'ARC-035')
      for (const name of ['compat-lease-check', 'compat-lease-generation']) {
        const path = 'deploy/' + name + '.json';
        if (get(path)) found.add(path);
      }
    if (!found.size) found.add('acceptance.json');
    return [...found]
      .map((path) => (existsSync(join(dir, path)) ? evidence(path) : inline(path)))
      .join(' · ');
  }
  const entries = [], correctionEntries = [];
  const localPage = (title, origin, path) => {
    const url = path ? new URL(path, origin) : new URL(origin);
    if (!['http:', 'https:'].includes(url.protocol) || !['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname) ||
        url.username || url.password || url.search || url.hash) throw Error('Invalid local walkthrough address');
    return '[' + title + '](' + url.href + ')';
  };
  for (const s of ['a', 'b']) {
    const cfg = env(s),
      w = get('manual/walkthrough-' + s + '.json');
    const requiredRelease = s === 'a' ? b?.source_archive_sha256 : selected?.inputs.sourceSHA256;
    const ready = requiredRelease && cfg?.walkthrough && cfg.releaseSHA256 === requiredRelease &&
      w?.releaseSHA256 === requiredRelease && w.execution?.launchID && cfg.launchID === w.execution.launchID &&
      (!sharedConsole || (w.sharedConsoleSHA256 === sharedConsole.console.sha256 &&
        cfg.sharedConsoleSHA256 === sharedConsole.console.sha256)) &&
      ['console', 'runtime', 'diagnostics'].every(key => cfg[key] === w[key]);
    const endpoint = ready
      ? [localPage('控制台', cfg.console), localPage('大屏', cfg.runtime), localPage('独立诊断', cfg.diagnostics)].join(' · ')
      : '启动后见 ' + inline('environment-' + s + '.json');
    if (ready && w.consoleCorrections?.projectPagination && w.consoleCorrections?.drafts) {
      const { projectPagination, drafts, importFocus } = w.consoleCorrections;
      correctionEntries.push('| ' + [
        s.toUpperCase(),
        localPage('11 个项目', cfg.console, projectPagination.openPath),
        localPage('成员', cfg.console, drafts.pages.members) + ' · ' +
          localPage('邀请', cfg.console, drafts.pages.invitations),
        localPage('操作人授权', cfg.console, drafts.pages.operatorAuthorization),
        importFocus ? localPage('监测导入', cfg.console, new URL(importFocus.listPath, cfg.console).pathname) : '待准备',
      ].join(' | ') + ' |');
    }
    entries.push(
      '| ' +
        [
          s.toUpperCase(),
          endpoint,
          ready ? evidence('manual/walkthrough-' + s + '.json') : '等待本版走查实例',
        ].join(' | ') +
        ' |',
    );
  }
  const row = (arc) =>
    '| ' +
    [
      arc.id,
      suite(arc.id),
      cell(arc.input),
      cell(arc.expected),
      arc.status === 'skipped' ? status.skipped : status[arc.status] + '。' + actual(arc),
      links(arc),
      arc.status === 'skipped' ? status.skipped : '待走查',
    ].join(' | ') +
    ' |';
  const run = relative(root, dir),
    gates = ledger.arcs.filter((a) => a.status === 'passed').length;
  const staticAssets = get(visualPath('visual/static-assets.json'));
  const releaseOption = selected ? ' --release-dir ' + relative(root, selected.directory) : '';
  const required = ledger.arcs.filter(a => a.status !== 'skipped');
  const outcomes = required.filter(a => a.status !== 'passed').map(a => a.id);
  const result = [
    '# 系统架构重构自验与人工走查',
    closeout ? '**全量验收已按用户要求停止。** 完整通过 ' + gates + '／' + required.length +
      ' 项功能验收。未执行项目标记为「未验证」，实际失败和中止保留各自原始记录。' +
      'ARC-037、ARC-038 及其他纯性能项目按用户要求跳过。' +
      line('部署入口、针对性复验及已知限制', join(root, 'docs/verification/system-architecture-refactor-closeout.md')) :
    '**' +
      (ledger.passed ? '待验收' : '实施中') +
      '**。本轮软件自验通过 ' +
      gates +
      '／' + required.length + ' 项功能自验。ARC-037、ARC-038 按用户要求跳过。人工接受待用户确认。' +
      (outcomes.length ? '尚待通过的编号为 ' + outcomes.join('、') + '。' : '') +
      '本轮使用本机隔离环境，生产切换另行记录。',
    '冻结版为 ' +
      inline(ledger.baseline || '待冻结') +
      '，PG 由版本 58 升级到 59。' +
      (b
        ? '冻结清单记录 ' +
          number(Object.keys(b.sourceFiles || {}).length) +
          ' 份源码、' +
          number(Object.keys(b.staticFiles || {}).length) +
          ' 份静态文件和 ' +
          Object.keys(b.binaries || {}).length +
          ' 个运行程序。'
        : '') +
      '运行目录为 ' +
      inline(run) +
      '。' +
      [
        evidence('baseline.json'),
        evidence(ledger.candidateManifest ?? 'candidate.json'),
        evidence('versions.json'),
      ].join(' · '),
    selected &&
      '本轮选择的候选源树摘要为 ' +
        inline(selected.inputs.sourceSHA256) +
        '。各场景的快照记录实际装载的发行物。同一运行目录中，重新封存前的证据保留其原有摘要。' +
        (selected.comparison ? evidence(selected.comparison) : ''),
    archivedSource &&
      archivedSource.application_source_sha256 === selected?.inputs.sourceSHA256 &&
      '候选应用源码归档含 ' +
        number(archivedSource.files_count) +
        ' 个构建输入，压缩包 SHA-256 为 ' +
        inline(archivedSource.archive_sha256) +
        '。' +
        line('源码包', join(selected.directory, archivedSource.archive)) +
        ' · ' +
        line('逐文件摘要', join(selected.directory, 'application-source.json')),
    offline?.verified &&
      offline.application_source_sha256 === selected?.inputs.sourceSHA256 &&
      offline.role_sql_sha256 &&
      '离线发行物适用 ' +
        inline(offline.goos + '/' + offline.goarch) +
        '，含 ' +
        number(offline.file_count) +
        ' 份文件。发行清单 SHA-256 为 ' +
        inline(offline.manifest_sha256) +
        '。主机二进制、三个 App、角色授权 SQL 及保留的解析方法均可按清单复核。' +
        line('离线发行清单', join(dir, offline.release)) +
        ' · ' +
        evidence('deploy/offline-release.json'),
    staticAssets?.candidate === selected?.inputs.sourceSHA256 &&
      staticAssets?.passed &&
      '三个 App 已逐文件核对。控制台 ' + number(staticAssets.counts.console.candidate) + ' 份文件中有 ' +
        staticAssets.counts.console.identical +
        ' 份、大屏运行壳 ' +
        staticAssets.counts['display-runtime'].identical +
        ' 份静态资源与冻结版摘要相同。独立诊断的入口脚本包含新增事件原因、消费位置及耗时格式，页面和 Vite 清单引用实际构建出的文件。' +
        (staticAssets.diagnosticsStyle?.permittedAddition && staticAssets.diagnosticsStyle.fonts?.allIdentical
          ? '共享组件的布尔变量触发 Tailwind 生成一条暂停动画规则。移除这 ' +
            (staticAssets.diagnosticsStyle.candidate.bytes - staticAssets.diagnosticsStyle.original.bytes) +
            ' 字节后，CSS 与冻结版逐字节相同。' + staticAssets.diagnosticsStyle.fonts.count + ' 个字体的 SHA-256 均相同。'
          : '诊断样式及字体的比对结果见逐文件清单。') +
        evidence(visualPath('visual/static-assets.json')),
    sharedConsole && '两种后端用同一份修正控制台走查。静态目录含 ' +
      number(Object.keys(sharedConsole.console.files).length) + ' 份文件，整份清单摘要为 ' +
      inline(sharedConsole.console.sha256) + '。共用前端的源码差异、冻结版失败记录与逐页证据分别留存。' +
      evidence('shared-console.json') + ' · ' + evidence('regression/browser-diagnoses/governance-pagination.json') +
      ' · ' + evidence('regression/browser-diagnoses/tenant-membership-focus-diagnosis.json'),
    originalBrowser?.cases === 295 && originalBrowser.missing?.length === 0 && originalBrowser.duplicates?.length === 0 &&
      '冻结前端首轮登记 ' + number(originalBrowser.cases) + ' 个浏览器用例，其中 ' +
      originalBrowser.counts.passed + ' 个通过、' + originalBrowser.counts.failed + ' 个断言失败、' +
      originalBrowser.counts.timedOut + ' 个超过原期限、' + originalBrowser.counts.skipped +
      ' 个按原测试条件跳过。原始分组、失败轨迹和截图均保留在该轮清单。' +
      evidence(originalBrowserPath),
    consoleCorrections?.backendSourceAfter === selected?.inputs.sourceSHA256 &&
      storageRace?.source_release === consoleCorrections.backendSourceBefore &&
      storageRace?.after?.native_exit === 0 && storageRace.after.races.length === 0 &&
      '上一候选版在多个角色冷启动时出现数据竞态。Go 存储连接器会原地修改共享的 ClickHouse 副本地址数组。本版先复制地址，再在各实例内规范化。原冷启动用例和新增的配置隔离用例均在竞态检测器下各连续通过三次。保留了原全量回归第 82 批失败、独立复现，以及两份源码的摘要。' +
      evidence('worker-checks/monitoring-storage-race/verified.json') + ' · ' + evidence('console-corrections/release.json'),
    firstSharedBrowser?.discovered === 295 && '冻结后端首次搭配修正控制台时，原浏览器回归发现 295 例，' +
      firstSharedBrowser.originalCounts.passed + ' 例通过、' + firstSharedBrowser.originalCounts.failed +
      ' 例断言失败、' + firstSharedBrowser.originalCounts.timedOut + ' 例超时、' +
      firstSharedBrowser.originalCounts.skipped + ' 例按原用例条件跳过。逐例索引保留了当时的轨迹、原断言和后续复验依据。' +
      evidence(firstSharedBrowserPath),
    activity?.baseline_source === b?.source_archive_sha256 && '冻结版和候选 ' +
      inline(activity.selected_source.slice(0, 12)) +
      ' 在数据库时钟稍慢时，都曾把尚未写入的用户活动标记成已保存。冻结版使用真实服务时钟也复现了这一现象。两版的受控用例清理会话缓存并重连后，最近活动回到了约 119 分钟前。原始条件、30 秒定时任务和撤销等反例可逐项回查。' + evidence(activityPath),
    sourceFocus?.observedSharedTimingFailure && '原控制台用同一批次列表在两种后端重放。列表先返回时，按 Esc 后均能回到原文件。原详情关闭后列表才返回时，两边的焦点都留在搜索框。' + evidence(sourceFocusPath),
    originalStorage?.passed && '并行构建期间，本机原监测契约环境的两个 ClickHouse 容器因 Docker 内存不足退出。两台均已使用原容器和原卷恢复，并核对了副本、' +
      number(Object.keys(originalStorage.frozen_tables).length) + ' 张冻结表以及 ' + number(originalStorage.s3_objects) +
      ' 个对象的内容摘要。原始故障与恢复证据可单独查阅。' + evidence('manual/original-storage-oom-recovery.json'),
    responseProof?.verified_application_source === selected?.inputs.sourceSHA256 &&
      responseProof?.functional_test_results?.length === 3 &&
      responseProof.functional_test_results.every((test) => test.state === 'pass') &&
      responseProof.pg_statement_timeout_observed?.length &&
      '旧候选在原 18,000 帧接入试验中有一帧未完成。原文和识别结果都可回读，数据库写入响应证据时超时，被记为不可重试。修复版在真实数据库超时以及解复用、解析阶段分别中断对象存储后，按原代次重新领取并完成。冻结样本、先前失败和三项实体测试均可回查。' +
        evidence('worker-checks/response-staging-fix/verified.json'),
    '## 重放与启动',
    '在仓库根目录执行 ' +
      inline('pnpm test:architecture') +
      ' 生成独立快照，运行路由、视觉、业务等价、故障和恢复套件以及全域功能回归。表中的套件名可用于 ' +
      inline('pnpm test:architecture <套件> --run-dir ' + run + releaseOption) +
      '。' +
      inline('--resume') +
      ' 可接续同一发行物的功能回归和截图。每条原回归命令的退出码、时长和日志在 ' +
      evidence('regression/') +
      '。' +
      '运行总账保留逐项判断。' +
      evidence('acceptance.json'),
    'ARC-037 的九轮接入压测、ARC-038 的十八场监测与大屏负载对照以及 ARC-039 中的纯性能、压力和长稳测试均登记为「按用户要求跳过」。已运行的负载结果保留各自的发行物、输入和原始结论。' +
      evidence('scope/functional-acceptance.json') + ' · ' + evidence('ingestion/campaigns/') + ' · ' + evidence('load/display/campaigns/'),
    closeout ? '本轮两侧回归队列均已关闭。逐项清单区分通过、实际失败、中止与未验证。' +
      evidence('scope/user-closeout.json') + ' · ' + evidence('regression/queue.json') :
    '两侧原回归的逐项结果和失败的原始用例在 ' +
      evidence('regression/triage.json') +
      '。断点顺序和共享端口在 ' +
      evidence('regression/queue.json') +
      '。运行 ' +
      inline('node scripts/architecture-regression-queue.mjs ' + run + ' baseline') +
      ' 或把末项改为 ' +
      inline('candidate') +
      ' 可以接续该侧尚未通过的命令。浏览器用例按冻结发行物的完整清单分 24 组执行。全部分组成功且每一原用例各有一次最终结果，才计为通过。',
    Number.isInteger(originalDiagnostics?.exitCode) &&
      pagination?.source === b?.source_archive_sha256 &&
      '冻结版原样运行诊断总命令，退出码为 ' +
        originalDiagnostics.exitCode +
        '。三种随包模板与同版角色目录生成的清单存在差异。筛选后台进程时，原测试期望“' +
        pagination.expectedFrozenText +
        '”，实际页面显示“' +
        pagination.actualPageText +
        '”。' +
        (currentDiagnostics && pairedDiagnostics.common
          ? '按各版实际角色目录重放后，两侧都通过 ' +
            pairedDiagnostics.common.faults.length +
            ' 个故障用例和 ' +
            pairedDiagnostics.common.browser.length +
            ' 个浏览器用例。'
          : '两侧按各自实际角色目录重放，结果单独登记。') +
        evidence('regression/baseline/test-diagnostics-unmodified.json') +
        ' · ' +
        evidence('regression/baseline/frozen-diagnostics-templates/comparison.json') +
        ' · ' +
        evidence('regression/baseline/frozen-diagnostics-browser/worker-pagination.json') +
        (currentDiagnostics && pairedDiagnostics.common ? ' · ' + evidence('diagnostics-comparison.json') : ''),
    '接入和解析继续验证收据、原文、重试和终态。监测继续验证对象服务真实停启后的数据读回，大屏继续验证人员、终端、SDK 和成员上限。大屏原命令选择 ' +
      inline('--boundary') + ' 核对 50,000 个成员的完整总数和 50,001 个成员时的拒绝结果。' + evidence('regression/'),
    membership?.passed && membership.mode === 'both-matched-after-frozen-timeout' &&
      membership.defaults.baseline.source === b?.source_archive_sha256 && membership.defaults.candidate.source === selected?.inputs.sourceSHA256 &&
      '大屏成员上限在候选版默认配置下通过。冻结版在预览 50,000 个成员时触及原查询时限，PostgreSQL 返回 57014。两端随后使用相同的 enable_nestloop=off、jit=off 运行原命令。每次事务保留 10 秒查询和 5 秒锁等待预算，预览总数均为 50,000，运行时读回 49,999 个测点，新增第 50,001 个成员后两处均拒绝。' +
      evidence(membership.defaults.baseline.file) + ' · ' + evidence(membership.defaults.candidate.file) + ' · ' + evidence(membership.evidencePath),
    '分别在两个终端执行以下命令可重新打开保留的 A、B 数据。启动时会校验角色和依赖，已准备的数据可继续走查。',
    '\u0060\u0060\u0060sh\nnode scripts/architecture-serve.mjs ' +
      run +
      ' a --walkthrough\nnode scripts/architecture-serve.mjs ' +
      run +
      ' b --walkthrough' +
      releaseOption +
      '\n\u0060\u0060\u0060',
    '| 环境 | 控制台、大屏、独立诊断 | 业务样本索引 |\n| --- | --- | --- |\n' + entries.join('\n'),
    correctionEntries.length > 0 && '| 环境 | 项目治理分页 | 草稿操作页 | 管理员第二窗口 | 批次回焦 |\n| --- | --- | --- | --- | --- |\n' + correctionEntries.join('\n'),
    '合成管理员、普通人员、另一租户管理员的凭据在 ' +
      inline('private/accounts.json') +
      '。受限项目成员在 ' +
      inline('private/manual-project-account.json') +
      '，各侧诊断账号的相对路径见样本索引的 ' +
      inline('credentials.diagnostics') +
      '。' +
      (sharedConsole ? '草稿测试的两位合成用户分别是办理人和租户管理员，凭据位置见各侧样本索引的 ' +
        inline('credentials.drafts') + '。目标成员、待办邀请和可切换角色的显示名见 ' + inline('consoleCorrections.drafts') + '。' : '') +
      '通过当前工作区的受控运行目录取用。恢复前后的数据清单见 ' +
      evidence('snapshot/manifest.json') +
      '，设计和切流操作见 ' +
      line('运维手册', join(root, 'docs/operations/system-architecture-refactor.md')) +
      '。',
    '## 逐项验证',
    '每个配对 HTTP 沙箱从同一冻结快照建库。' +
      inline('snapshot.json') +
      ' 记录 PG 归档摘要、列存初始行数、对象数和当次安装的角色授权文件摘要。关闭回放后，' +
      inline('storage-archive.json') +
      ' 保存 ClickHouse 表清单和逐表重新挂载语句，便于复核原始行。人工走查样本使用各自持续保留的数据库。',
    '配对 HTTP 工作流保存每次请求的操作、响应码、两侧响应原始字节摘要和字段差异。新记录用 ' +
      inline('mappingReasons') +
      ' 写明生成值的逐字段配对原因。双向登记的对象 ID 可用于后续请求，分页游标须指向同一快照与边界行，并在各侧实际翻页。CSV 下载先核对各自清单里的长度和 SHA-256，再逐字节比较替换已登记标识后的内容。' +
      line('工作流比较实现', join(root, 'scripts/architecture-workflow-http.mjs')),
    '解析结果中的 service_build_digest 标识实际执行该请求的解析服务。自动和人工解析回放分别校验原始响应中的该字段与该侧封存的 parser 二进制 SHA-256。两侧继续逐项比较方法版本、输入摘要、原文字节、结果和数值精度。' +
      line('解析回放及构建摘要核对', join(root, 'scripts/architecture-ingestion-http.mjs')),
    '下表按规格原始输入和预期列出实际结果。每个证据文件记录对应运行的用例、对照字段或读回记录。完整门槛以总账为准。',
    '| 编号 | 套件 | 输入与动作 | 可观察预期 | 实测 | 证据 | 人工 |\n| --- | --- | --- | --- | --- | --- | --- |\n' +
      ledger.arcs.map(row).join('\n'),
    '## 已确认的兼容与修复决定',
    '用户于 2026-11-19 确认保留租户管理员候选搜索的冻结行为。未指定租户时，候选项含 ID、用户名、显示名、被替换角色，以及固定的 `has_avatar: false` 和 `avatar_revision: ""`。冻结 OpenAPI 未列出这两个占位字段。本次对两版采用同一组精确字段和值断言，并保留允许和拒绝用例。' +
      line('决定与适用范围', join(root, 'docs/development/system-architecture-refactor-spec.md')) +
      (get('regression/browser-diagnoses/tenant-candidate-authorization.json') ?
        ' · ' + evidence('regression/browser-diagnoses/tenant-candidate-authorization.json') : '') +
      (get('http/scenarios/governance-roles.json')?.records?.some(r => r.scenario === 'unscoped_tenant_admin_candidate') ?
        ' · ' + evidence('http/scenarios/governance-roles.json') : ''),
    '用户批准修复会话活动持久化。数据库时钟到达且会话有效时才能保存活动，撤销、两小时空闲和十二小时绝对上限继续生效。' +
      activityResults.filter(path => get(path)).map(evidence).join(' · '),
    sharedConsole?.beforeImportFocus && '用户批准了共用控制台的三项修复，分别为项目治理分页、成员与邀请弹窗的草稿复核，以及监测导入详情的回焦。保留冻结前端和前两项修正后的发行物。' +
      evidence('shared-console.json') + (correctedFocus?.passed && correctedFocus.consoleSHA256 === sharedConsole.console.sha256 ? ' · ' + evidence(focusPath) : ''),
    ...(visualFindings.length ? ['## 冻结页面实测', ...visualFindings] : []),
    ...(ledger.scope?.scope === 'functional' ? [] : performance),
    '## 人工走查',
    '1. 管理员在两侧登录，查 MFA、成员、头像和账号中心。用浏览器后退核对筛选、页码、焦点和权限按钮。\n' +
      '2. 工程管理者从设施树定位断面、孔位、测点并查看设备输出及使用历史。受限项目成员可以读取已授权通道的 10 条观测，跨项目访问按原规则受限。\n' +
      '3. 接入人员按样本索引查原始收据、原文字节和解析结果。监测人员显式发布样表后核对趋势、单位、质量和谱系，并切换两次历史分析版本。\n' +
      '4. 资料管理者从第 2 片续传预置附件，读回已完成附件，预览模型和图纸。值班人员从同一条通知跳转到任务，并在人员大屏和固定终端核对发布结果。原始分片、摘要与终端测试材料保存在各侧私有的 ' +
      inline('walkthrough-assets/manifest.json') +
      '。\n' +
      '5. 运维人员分别停启 parser、ClickHouse 连接和 B 的消息代理，核对业务边界、积压和独立诊断。' +
      inline('node scripts/architecture-walkthrough-control.mjs ' + run + ' b parser stop') +
      ' 暂停解析，' +
      inline('parser start') +
      ' 恢复。支持相同语法的 ' +
      inline('clickhouse') +
      ' 和 ' +
      inline('broker') +
      '。两侧重放使用独立诊断登录，记录连接中断、当班人员读取设施台账和恢复的实际探针结果。' +
      evidence('manual/operator-a.json') +
      ' · ' +
      evidence('manual/operator-b.json') +
      '。备份恢复与切回后已受理事项的证据位于 ' +
      evidence('deploy/') +
      '。',
    sharedConsole && '在两侧分别打开一个含至少 11 个项目的测试租户，进入“项目治理”，选择每页 10 项。在 375、768、1024 和 1440 像素宽的窗口及两种主题下，通过鼠标和 Tab 键操作每页数量及前后翻页。第 2 页应可见余下项目，返回时第 1 页的十个项目应按原序出现，页面无需横向滚动。' +
      (get('console-corrections/pagination.json')?.passed ? evidence('console-corrections/pagination.json') : ''),
    sharedConsole && '用具备成员与邀请办理权限的合成账号打开“暂停成员”或“重发邀请”，填写原因后切到另一个窗口再返回。资格复核通过后应看到原文。另一个管理员撤销操作账号的相应资格后，再切回窗口，原操作应消失，目标成员和邀请维持原状态。恢复资格并重新打开时原因应为空。' +
      (get('console-corrections/drafts.json')?.passed ? evidence('console-corrections/drafts.json') : ''),
    sharedConsole?.beforeImportFocus && '在各侧样本索引的 `consoleCorrections.importFocus` 读取搜索前缀和目标文件名。进入监测导入，用前缀筛出 23 条样本，按每页 10 条翻到第二页，打开目标文件的处理记录。刷新详情后按 Esc，原行返回时应获得焦点。在原行返回前按 Tab 或点击搜索框并输入，焦点和输入应保留。如果筛选后原行已不可见，焦点应停在当前控件。' +
      (correctedFocus?.passed && correctedFocus.consoleSHA256 === sharedConsole.console.sha256 ? line('独立回放',join(dir,focusPath)) : '') +
      (currentRetainedFocus ? ' · ' + line('两侧走查实例实测',join(dir,retainedFocusPath)) : ''),
    reviewMatrices.length
      ? '已生成的对照矩阵可按页面编号定位截图及差异图。' + reviewMatrices.map(evidence).join(' · ')
      : '本版完整截图对照矩阵未生成。' + (closeout ? evidence('scope/user-closeout.json') : ''),
  ];
  writeFileSync(destination, result.filter(Boolean).join('\n\n') + '\n', {
    mode: 0o600,
  });
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const dir = runDirectory(process.argv[2]);
  const target =
    process.argv[3] === '--publish'
      ? join(root, 'docs/verification/system-architecture-refactor.md')
      : join(dir, 'verification.md');
  renderVerification(dir, read(join(dir, 'acceptance.json')), target);
  console.log('Rendered ' + basename(target));
}
