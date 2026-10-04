import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../core/app_locale_context.dart';
import '../core/dashboard_sample.dart';
import '../state/app_state.dart';
import 'analytics_page.dart';
import 'widgets.dart';

class DashboardPage extends StatefulWidget {
  const DashboardPage({super.key, this.onNavigate});
  final ValueChanged<String>? onNavigate;

  @override
  State<DashboardPage> createState() => _DashboardPageState();
}

class _DashboardPageState extends State<DashboardPage> with WidgetsBindingObserver {
  Map<String, dynamic> status = {};
  Map<String, dynamic> online = {};
  List<dynamic> logs = [];
  final samples = <DashboardSample>[];
  Timer? timer;
  DateTime? updatedAt;
  DateTime? lastDetails;
  bool refreshing = false;
  bool restarting = false;
  bool foreground = true;
  String? error;
  String? logsError;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    final bootstrap = context.read<AppState>().bootstrap;
    status = _map(bootstrap['status']);
    online = _map(_map(bootstrap['panel'])['onlines']);
    timer = Timer(Duration.zero, refresh);
  }

  @override
  void dispose() {
    timer?.cancel();
    WidgetsBinding.instance.removeObserver(this);
    super.dispose();
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    foreground = state == AppLifecycleState.resumed;
    timer?.cancel();
    if (foreground) {
      samples.clear();
      lastDetails = null;
      refresh();
    }
  }

  Future<void> refresh() async {
    if (refreshing || !mounted || !foreground) return;
    timer?.cancel();
    setState(() => refreshing = true);
    final api = context.read<AppState>().api;
    try {
      if (api == null) return;
      final next = _map(await api.get('status', query: {
        'request': 'cpu,mem,dsk,swp,net,sbd,db${updatedAt == null ? ',sys' : ''}',
      }));
      if (!mounted || !foreground || api != context.read<AppState>().api) return;
      final now = DateTime.now();
      setState(() {
        status = {...status, ...next};
        samples.add(DashboardSample.fromStatus(next, now, samples.lastOrNull));
        if (samples.length > 60) samples.removeAt(0);
        updatedAt = now;
        error = null;
      });
      if (lastDetails == null || now.difference(lastDetails!).inSeconds >= 10) {
        lastDetails = now;
        // The core may be stopped while the status and logs remain available.
        final results = await Future.wait([
          api.get('onlines').then<dynamic>((value) => value, onError: (_) => null),
          api.get('logs', query: {'limit': 8, 'level': 'ALL'})
              .then<dynamic>((value) => value, onError: (Object e) => e),
        ]);
        if (!mounted || !foreground || api != context.read<AppState>().api) return;
        setState(() {
          online = _map(results[0]);
          if (results[1] is Map) {
            logs = List<dynamic>.from(_map(results[1])['items'] as List? ?? const []);
            logsError = null;
          } else {
            logsError = results[1].toString();
          }
        });
      }
    } catch (exception) {
      if (mounted) setState(() => error = exception.toString());
    } finally {
      if (mounted) {
        setState(() => refreshing = false);
        if (foreground) timer = Timer(const Duration(seconds: 2), refresh);
      }
    }
  }

  Future<void> restartCore() async {
    if (restarting) return;
    if (!await confirm(context, title: context.tr('dashboard.restartCore'),
        message: context.tr('dashboard.restartConfirm'), action: context.tr('dashboard.restartCore'))) return;
    if (!mounted) return;
    setState(() => restarting = true);
    try {
      await context.read<AppState>().api!.post('actions/restart-core');
      if (!mounted) return;
      showMessage(context, context.tr('dashboard.restartCompleted'));
      lastDetails = null;
      await refresh();
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    } finally {
      if (mounted) setState(() => restarting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final core = _map(status['sbd']);
    final stats = _map(core['stats']);
    final system = _map(status['sys']);
    final database = _map(status['db']);
    final running = core['running'] == true;
    final colors = Theme.of(context).colorScheme;
    final latest = samples.lastOrNull;
    final stamp = updatedAt == null ? '—' : TimeOfDay.fromDateTime(updatedAt!).format(context);
    return RefreshIndicator(
      onRefresh: refresh,
      child: LayoutBuilder(builder: (context, constraints) {
        final wide = constraints.maxWidth >= 850;
        return ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          padding: const EdgeInsets.fromLTRB(12, 8, 12, 24),
          children: [
            PageHeader(title: context.t('dashboard.title'),
              subtitle: context.t('dashboard.updatedAt', args: {'time': stamp}),
              actions: [IconButton.filledTonal(tooltip: context.t('common.refresh'),
                onPressed: refreshing ? null : refresh, icon: const Icon(Icons.refresh))]),
            if (error != null) _notice(context.t('dashboard.refreshFailed', args: {'error': error})),
            if (refreshing && updatedAt == null) const LinearProgressIndicator(),
            _columns(wide, [
              _section('sing-box', Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Wrap(spacing: 8, runSpacing: 8, crossAxisAlignment: WrapCrossAlignment.center, children: [
                  Icon(Icons.power_settings_new, color: running ? Colors.green : colors.error),
                  Text(context.t(restarting ? 'dashboard.restarting' : core.isEmpty ? 'dashboard.loading' : running ? 'dashboard.running' : 'dashboard.stopped'),
                    style: Theme.of(context).textTheme.headlineSmall),
                ]),
                const SizedBox(height: 16),
                Wrap(spacing: 24, runSpacing: 12, children: [
                  _detail(context.t('dashboard.uptime'), _duration(stats['Uptime'])),
                  _detail(context.t('dashboard.memory'), formatBytes(stats['Alloc'])),
                  _detail(context.t('dashboard.threads'), '${stats['NumGoroutine'] ?? '—'}'),
                ]),
                const SizedBox(height: 16),
                Wrap(spacing: 8, runSpacing: 8, children: [
                  FilledButton.tonalIcon(onPressed: restarting ? null : restartCore,
                    icon: const Icon(Icons.restart_alt), label: Text(context.t('dashboard.restartCore'))),
                  TextButton(onPressed: () => widget.onNavigate?.call('nav.config'), child: Text(context.t('nav.config'))),
                ]),
              ])),
              _grid(2, [
                _summary('nav.clients', database['clients'], Icons.people_outline,
                  context.t('dashboard.onlineUsers', args: {'count': _strings(online['user']).length})),
                _summary('nav.inbounds', database['inbounds'], Icons.login, context.t('navigation.access')),
                _summary('nav.outbounds', database['outbounds'], Icons.logout, context.t('navigation.network')),
                _summary('nav.endpoints', database['endpoints'], Icons.vpn_key_outlined, 'WireGuard · WARP · Tailscale'),
              ]),
            ]),
            const SizedBox(height: 12),
            _grid(wide ? 4 : constraints.maxWidth < 360 ? 1 : 2, [
              _metric('CPU', '${_number(status['cpu']).toStringAsFixed(1)}%', _number(status['cpu']) / 100, Icons.memory),
              for (final entry in const {'mem': 'dashboard.memory', 'dsk': 'dashboard.disk', 'swp': 'dashboard.swap'}.entries)
                _metric(context.t(entry.value), '${formatBytes(_map(status[entry.key])['current'])} / ${formatBytes(_map(status[entry.key])['total'])}',
                  _ratio(_map(status[entry.key])), entry.key == 'dsk' ? Icons.storage_outlined : Icons.data_usage),
            ]),
            const SizedBox(height: 12),
            _columns(wide, [
              _section(context.t('dashboard.networkTrend'), Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Wrap(spacing: 16, runSpacing: 8, children: [
                  Text('↑ ${formatBytes(latest?.uploadRate)}/s', style: const TextStyle(color: Colors.orange)),
                  Text('↓ ${formatBytes(latest?.downloadRate)}/s', style: const TextStyle(color: Colors.green)),
                ]),
                _chart(samples.map((s) => s.uploadRate).toList(), second: samples.map((s) => s.downloadRate).toList()),
                Text('${context.t('dashboard.totalUpload')}: ${formatBytes(database['clientUp'])} · ${context.t('dashboard.totalDownload')}: ${formatBytes(database['clientDown'])}'),
              ])),
              _section(context.t('dashboard.cpuTrend'), Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Text('${_number(status['cpu']).toStringAsFixed(1)}%'),
                _chart(samples.map((s) => s.cpu).toList(), maximum: 100),
                Text(context.t('dashboard.historyHint')),
              ])),
            ]),
            const SizedBox(height: 12),
            _columns(wide, [
              _section(context.t('dashboard.recentLogs'), Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                if (logsError != null) Text(logsError!, style: TextStyle(color: colors.error)),
                if (logs.isEmpty && logsError == null) Text(context.t('logs.empty')),
                for (final raw in logs) _log(_map(raw)),
                TextButton.icon(onPressed: () => Navigator.of(context).push(MaterialPageRoute<void>(
                  builder: (_) => Scaffold(appBar: AppBar(title: Text(context.t('logs.title'))), body: const AnalyticsPage(initialTab: 3)))),
                  icon: const Icon(Icons.receipt_long_outlined), label: Text(context.t('dashboard.viewLogs'))),
              ])),
              Column(children: [
                _section(context.t('dashboard.systemInfo'), Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                  _info(context.t('dashboard.host'), '${system['hostName'] ?? '—'}'),
                  _info('S-UI Next', 'v${system['appVersion'] ?? '—'}'),
                  _info('CPU', '${system['cpuCount'] ?? '—'} · ${system['cpuType'] ?? '—'}'),
                  _info(context.t('dashboard.uptime'), system['bootTime'] == null ? '—' : _duration(DateTime.now().millisecondsSinceEpoch / 1000 - _number(system['bootTime']))),
                ])),
                const SizedBox(height: 8),
                _section(context.t('dashboard.quickActions'), Column(children: [
                  _shortcut('tools.backupRestore', 'nav.tools', Icons.backup_outlined),
                  _shortcut('nav.analytics', 'nav.analytics', Icons.query_stats),
                  _shortcut('nav.tools', 'nav.tools', Icons.build_outlined),
                ])),
              ]),
            ]),
            const SizedBox(height: 12),
            _section(context.t('dashboard.onlineStatus'), Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              for (final entry in const {'user': 'dashboard.users', 'inbound': 'dashboard.inbounds', 'outbound': 'dashboard.outbounds'}.entries)
                Padding(padding: const EdgeInsets.only(bottom: 10), child: Wrap(spacing: 8, runSpacing: 6,
                  crossAxisAlignment: WrapCrossAlignment.center, children: [
                    Text(context.t(entry.value)),
                    if (_strings(online[entry.key]).isEmpty) Text(context.t('dashboard.noOnline')),
                    for (final name in _strings(online[entry.key])) Chip(label: Text(name)),
                  ])),
            ])),
          ],
        );
      }),
    );
  }

  Widget _notice(String text) => Card(color: Theme.of(context).colorScheme.errorContainer,
    child: Padding(padding: const EdgeInsets.all(12), child: Text(text)));

  Widget _columns(bool wide, List<Widget> children) => wide
      ? Row(crossAxisAlignment: CrossAxisAlignment.start, children: [
          Expanded(child: children[0]), const SizedBox(width: 12), Expanded(child: children[1])])
      : Column(children: [children[0], const SizedBox(height: 12), children[1]]);

  Widget _grid(int columns, List<Widget> children) => LayoutBuilder(builder: (context, box) => Wrap(
    spacing: 10, runSpacing: 10, children: [
      for (final child in children) SizedBox(width: (box.maxWidth - 10 * (columns - 1)) / columns, child: child),
    ]));

  Widget _section(String title, Widget child) => Card(margin: EdgeInsets.zero,
    child: Padding(padding: const EdgeInsets.all(16), child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
      Text(title, style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700)),
      const SizedBox(height: 12), child,
    ])));

  Widget _detail(String title, String value) => Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
    Text(title, style: Theme.of(context).textTheme.bodySmall), Text(value),
  ]);

  Widget _summary(String label, dynamic count, IconData icon, String detail) => Card(margin: EdgeInsets.zero,
    child: InkWell(borderRadius: BorderRadius.circular(20), onTap: () => widget.onNavigate?.call(label),
      child: Padding(padding: const EdgeInsets.all(14), child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Icon(icon, color: Theme.of(context).colorScheme.primary), const SizedBox(height: 8),
        Text(context.t(label)),
        Text('${count ?? 0}', style: Theme.of(context).textTheme.headlineMedium),
        Text(detail, style: Theme.of(context).textTheme.bodySmall),
      ]))));

  Widget _metric(String title, String value, double fraction, IconData icon) => Card(margin: EdgeInsets.zero,
    child: Padding(padding: const EdgeInsets.all(14), child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Icon(icon, color: Theme.of(context).colorScheme.primary), const SizedBox(height: 8),
      Text(title), const SizedBox(height: 4), Text(value), const SizedBox(height: 12),
      LinearProgressIndicator(value: fraction.clamp(0, 1)),
    ])));

  Widget _info(String title, String value) => Padding(padding: const EdgeInsets.only(bottom: 8),
    child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Text(title, style: Theme.of(context).textTheme.labelMedium), SelectableText(value),
    ]));

  Widget _shortcut(String label, String target, IconData icon) => ListTile(
    contentPadding: EdgeInsets.zero, leading: Icon(icon), title: Text(context.t(label)),
    onTap: () => widget.onNavigate?.call(target));

  Widget _log(Map<String, dynamic> entry) {
    final level = entry['level']?.toString() ?? 'INFO';
    final color = level == 'ERROR' ? Colors.red : level == 'WARNING' ? Colors.orange : Theme.of(context).colorScheme.primary;
    return Container(margin: const EdgeInsets.only(bottom: 10), padding: const EdgeInsetsDirectional.only(start: 10),
      decoration: BoxDecoration(border: BorderDirectional(start: BorderSide(color: color, width: 3))),
      child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
        Text('${entry['time'] ?? formatTimestamp(entry['timestamp'])} · $level', style: Theme.of(context).textTheme.labelSmall),
        Text(entry['message']?.toString() ?? '', maxLines: 4, overflow: TextOverflow.ellipsis,
          style: const TextStyle(fontFamily: 'monospace', fontSize: 12)),
      ]));
  }

  Widget _chart(List<double> values, {List<double> second = const [], double? maximum}) => SizedBox(
    height: 130, width: double.infinity,
    child: values.length < 2 ? Center(child: Text(context.t('dashboard.collecting')))
      : Padding(padding: const EdgeInsets.symmetric(vertical: 16), child: CustomPaint(painter: _TrendPainter(
          values, second, maximum, Theme.of(context).colorScheme.primary, Theme.of(context).colorScheme.outlineVariant))),
  );
}

class _TrendPainter extends CustomPainter {
  _TrendPainter(this.values, this.second, this.maximum, this.primary, this.grid);
  final List<double> values, second;
  final double? maximum;
  final Color primary, grid;

  @override
  void paint(Canvas canvas, Size size) {
    final top = maximum ?? math.max(1.0, [...values, ...second].fold(0.0, math.max));
    final gridPaint = Paint()..color = grid..strokeWidth = 1;
    for (var i = 0; i < 3; i++) {
      final y = size.height * i / 2;
      canvas.drawLine(Offset(0, y), Offset(size.width, y), gridPaint);
    }
    void line(List<double> points, Color color) {
      if (points.length < 2) return;
      final path = Path();
      for (var i = 0; i < points.length; i++) {
        final x = size.width * i / (points.length - 1);
        final y = size.height * (1 - (points[i] / top).clamp(0, 1));
        if (i == 0) { path.moveTo(x, y); } else { path.lineTo(x, y); }
      }
      canvas.drawPath(path, Paint()..color = color..strokeWidth = 2..style = PaintingStyle.stroke);
    }
    line(values, second.isEmpty ? primary : Colors.orange);
    line(second, Colors.green);
  }

  @override
  bool shouldRepaint(covariant _TrendPainter oldDelegate) => true;
}

Map<String, dynamic> _map(dynamic value) => value is Map ? Map<String, dynamic>.from(value) : {};
List<String> _strings(dynamic value) => value is List ? value.map((item) => item.toString()).toList() : [];
double _number(dynamic value) => double.tryParse(value?.toString() ?? '') ?? 0;
double _ratio(Map<String, dynamic> value) => _number(value['total']) <= 0 ? 0 : _number(value['current']) / _number(value['total']);
String _duration(dynamic seconds) {
  final duration = Duration(seconds: _number(seconds).clamp(0, double.maxFinite).toInt());
  return '${duration.inDays}d ${duration.inHours % 24}h ${duration.inMinutes % 60}m';
}
