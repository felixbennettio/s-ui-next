import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../core/app_locale_context.dart';
import '../core/connection_display.dart';
import '../state/app_state.dart';
import 'widgets.dart';

class AnalyticsPage extends StatefulWidget {
  const AnalyticsPage({super.key, this.initialTab = 0});

  final int initialTab;

  @override
  State<AnalyticsPage> createState() => _AnalyticsPageState();
}

class _AnalyticsPageState extends State<AnalyticsPage> with SingleTickerProviderStateMixin {
  late final TabController tabs;
  final search = TextEditingController();
  final user = TextEditingController();
  DateTime start = DateTime.now().subtract(const Duration(days: 7));
  DateTime end = DateTime.now();
  String resource = 'user';
  String level = 'ALL';
  bool loading = false;
  List<dynamic> usageItems = [];
  List<dynamic> statItems = [];
  List<dynamic> logItems = [];
  Map<String, dynamic> connectionData = {};
  int usageTotal = 0;
  int logTotal = 0;
  int upload = 0;
  int download = 0;
  String? error;
  final resolvingAddresses = <String>{};
  final resolvedAddresses = <String>{};

  @override
  void initState() {
    super.initState();
    tabs = TabController(length: 4, initialIndex: widget.initialTab, vsync: this)..addListener(() {
        if (!tabs.indexIsChanging) load();
      });
    load();
  }

  @override
  void dispose() {
    tabs.dispose();
    search.dispose();
    user.dispose();
    super.dispose();
  }

  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
      resolvingAddresses.clear();
      resolvedAddresses.clear();
    });
    try {
      final query = <String, dynamic>{
        'start': unixStartOfDay(start),
        'end': unixEndOfDay(end),
        'search': search.text.trim(),
        'limit': tabs.index == 0 ? 500 : tabs.index == 1 ? 2000 : 1000,
      };
      final api = context.read<AppState>().api!;
      if (tabs.index == 0) {
        if (user.text.trim().isNotEmpty) query['user'] = user.text.trim();
        final result = Map<String, dynamic>.from(await api.get('analytics/usage', query: query) as Map);
        if (mounted) {
          setState(() {
            usageItems = List<dynamic>.from(result['items'] as List? ?? const []);
            usageTotal = int.tryParse(result['total']?.toString() ?? '') ?? 0;
            upload = int.tryParse(result['upload']?.toString() ?? '') ?? 0;
            download = int.tryParse(result['download']?.toString() ?? '') ?? 0;
          });
        }
      } else if (tabs.index == 3) {
        query['level'] = level;
        query['user'] = user.text.trim();
        final result = Map<String, dynamic>.from(await api.get('logs', query: query) as Map);
        if (mounted) {
          setState(() {
            logItems = List<dynamic>.from(result['items'] as List? ?? const []);
            logTotal = int.tryParse(result['total']?.toString() ?? '') ?? 0;
          });
        }
      } else {
        query['resource'] = resource;
        if (user.text.trim().isNotEmpty) query['tag'] = user.text.trim();
        if (tabs.index == 1) {
          final result = Map<String, dynamic>.from(await api.get('analytics/stats', query: query) as Map);
          if (mounted) setState(() => statItems = List<dynamic>.from(result['items'] as List? ?? const []));
        } else {
          final result = Map<String, dynamic>.from(await api.get('analytics/connections', query: query) as Map);
          if (mounted) setState(() => connectionData = result);
        }
      }
    } catch (exception) {
      if (mounted) setState(() => error = exception.toString());
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> pickDate(bool isStart) async {
    final value = await showDatePicker(
      context: context,
      initialDate: isStart ? start : end,
      firstDate: DateTime(2020),
      lastDate: DateTime.now().add(const Duration(days: 1)),
    );
    if (value == null) return;
    if (!mounted) return;
    setState(() {
      if (isStart) {
        start = value;
        if (start.isAfter(end)) end = start;
      } else {
        end = value;
        if (end.isBefore(start)) start = end;
      }
    });
    load();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        PageHeader(title: context.t('analytics.title'), subtitle: context.t('analytics.subtitle')),
        TabBar(
          controller: tabs,
          tabs: [
            Tab(text: context.t('analytics.userUsage')),
            Tab(text: context.t('analytics.trafficTrends')),
            Tab(text: context.t('analytics.connections')),
            Tab(text: context.t('logs.title')),
          ],
        ),
        FilterCard(
          child: Column(
            children: [
              Row(
                children: [
                  Expanded(child: TextField(controller: search, onSubmitted: (_) => load(), decoration: InputDecoration(labelText: context.t('common.search'), prefixIcon: const Icon(Icons.search)))),
                  const SizedBox(width: 8),
                  IconButton.filledTonal(onPressed: loading ? null : load, icon: const Icon(Icons.refresh)),
                ],
              ),
              const SizedBox(height: 8),
              Row(
                children: [
                  Expanded(child: TextField(controller: user, onSubmitted: (_) => load(), decoration: InputDecoration(labelText: context.t('analytics.userExact'), prefixIcon: const Icon(Icons.person_search_outlined)))),
                  if (tabs.index != 0 && tabs.index != 3) ...[
                    const SizedBox(width: 8),
                    Expanded(
                      child: AnchoredSelect<String>(
                        value: resource,
                        label: context.t('analytics.resource'),
                        options: [
                          SelectOption('user', context.t('analytics.userUsage')),
                          SelectOption('inbound', context.t('analytics.inbounds')),
                          SelectOption('outbound', context.t('analytics.outbounds')),
                          SelectOption('endpoint', context.t('analytics.nodes')),
                          SelectOption('destination', context.t('analytics.destinations')),
                          SelectOption('all', context.t('common.all')),
                        ],
                        onChanged: (value) {
                          setState(() => resource = value);
                          load();
                        },
                      ),
                    ),
                  ] else if (tabs.index == 3) ...[
                    const SizedBox(width: 8),
                    Expanded(
                      child: AnchoredSelect<String>(
                        value: level,
                        label: context.t('logs.level'),
                        options: [
                          for (final value in const ['ALL', 'DEBUG', 'INFO', 'WARNING', 'ERROR']) SelectOption(value, value),
                        ],
                        onChanged: (value) {
                          setState(() => level = value);
                          load();
                        },
                      ),
                    ),
                  ],
                ],
              ),
              const SizedBox(height: 8),
              Row(
                children: [
                  Expanded(child: OutlinedButton.icon(onPressed: () => pickDate(true), icon: const Icon(Icons.calendar_today_outlined), label: Text('${context.t('common.from')} ${_date(start)}'))),
                  const SizedBox(width: 8),
                  Expanded(child: OutlinedButton.icon(onPressed: () => pickDate(false), icon: const Icon(Icons.event_outlined), label: Text('${context.t('common.to')} ${_date(end)}'))),
                ],
              ),
            ],
          ),
        ),
        if (loading) const LinearProgressIndicator(minHeight: 2),
        Expanded(
          child: error != null
              ? EmptyState(label: error!, icon: Icons.error_outline)
              : TabBarView(controller: tabs, children: [_usage(), _stats(), _connections(), _logs()]),
        ),
      ],
    );
  }

  Widget _usage() {
    return RefreshIndicator(
      onRefresh: load,
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(12, 0, 12, 24),
        children: [
          Row(
            children: [
              Expanded(child: _SummaryCard(label: context.t('analytics.upload'), value: formatBytes(upload), color: Colors.orange)),
              const SizedBox(width: 8),
              Expanded(child: _SummaryCard(label: context.t('analytics.download'), value: formatBytes(download), color: Colors.green)),
              const SizedBox(width: 8),
              Expanded(child: _SummaryCard(label: context.t('analytics.total'), value: formatBytes(upload + download), color: Colors.blue)),
            ],
          ),
          Padding(padding: const EdgeInsets.symmetric(vertical: 10, horizontal: 4), child: Text(context.t('analytics.usersCount', args: {'count': usageTotal}))),
          if (usageItems.isEmpty)
            EmptyState(label: context.t('analytics.noUsage'))
          else
            for (final raw in usageItems) _usageCard(Map<String, dynamic>.from(raw as Map)),
        ],
      ),
    );
  }

  Widget _usageCard(Map<String, dynamic> item) {
    final total = _int(item['total']);
    final quota = _int(item['quota']);
    final progress = quota > 0 ? (total / quota).clamp(0.0, 1.0).toDouble() : 0.0;
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(children: [Icon(item['online'] == true ? Icons.circle : Icons.circle_outlined, size: 12, color: item['online'] == true ? Colors.green : null), const SizedBox(width: 8), Expanded(child: Text(item['user']?.toString() ?? '—', style: const TextStyle(fontWeight: FontWeight.w700))), if (item['group']?.toString().isNotEmpty == true) Chip(label: Text(item['group'].toString()))]),
            const SizedBox(height: 10),
            Row(children: [Expanded(child: Text('↑ ${formatBytes(item['upload'])}', style: const TextStyle(color: Colors.orange))), Expanded(child: Text('↓ ${formatBytes(item['download'])}', style: const TextStyle(color: Colors.green))), Text(formatBytes(total), style: const TextStyle(fontWeight: FontWeight.w700))]),
            if (quota > 0) ...[
              const SizedBox(height: 10),
              LinearProgressIndicator(value: progress),
              const SizedBox(height: 4),
              Text(context.t('analytics.quota', args: {'quota': formatBytes(quota)}), style: Theme.of(context).textTheme.bodySmall),
            ],
            if (_int(item['expiry']) > 0) Padding(padding: const EdgeInsets.only(top: 5), child: Text(context.t('analytics.expiry', args: {'time': formatTimestamp(item['expiry'])}), style: Theme.of(context).textTheme.bodySmall)),
            Align(alignment: Alignment.centerRight, child: TextButton.icon(onPressed: () => _showConnectionDetails('user', item['user']?.toString() ?? ''), icon: const Icon(Icons.text_snippet_outlined), label: Text(context.t('common.details')))),
          ],
        ),
      ),
    );
  }

  Widget _stats() {
    if (statItems.isEmpty) return EmptyState(label: context.t('analytics.noStats'));
    final points = statItems.map((raw) => Map<String, dynamic>.from(raw as Map)).toList();
    final uploadPoints = points.where((item) => item['direction'] == true).toList();
    final downloadPoints = points.where((item) => item['direction'] != true).toList();
    return RefreshIndicator(
      onRefresh: load,
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(12, 0, 12, 24),
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.fromLTRB(12, 18, 12, 12),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('$resource · ${user.text.trim().isEmpty ? context.t('analytics.allTags') : user.text.trim()}', style: const TextStyle(fontWeight: FontWeight.w700)),
                  const SizedBox(height: 12),
                  SizedBox(height: 220, child: CustomPaint(painter: _TrafficPainter(uploadPoints, downloadPoints), child: const SizedBox.expand())),
                  const SizedBox(height: 8),
                  Row(mainAxisAlignment: MainAxisAlignment.center, children: [const Icon(Icons.circle, size: 10, color: Colors.orange), const SizedBox(width: 5), Text(context.t('analytics.upload')), const SizedBox(width: 16), const Icon(Icons.circle, size: 10, color: Colors.green), const SizedBox(width: 5), Text(context.t('analytics.download'))]),
                ],
              ),
            ),
          ),
          const SizedBox(height: 8),
          for (final item in points.reversed.take(100))
            ListTile(
              dense: true,
              leading: Icon(item['direction'] == true ? Icons.upload : Icons.download, color: item['direction'] == true ? Colors.orange : Colors.green),
              title: Text('${item['resource']} · ${item['tag']}'),
              subtitle: Text(formatTimestamp(item['dateTime'])),
              trailing: Text(formatBytes(item['traffic'])),
            ),
        ],
      ),
    );
  }

  Widget _connections() {
    final items = List<dynamic>.from(connectionData['items'] as List? ?? const []);
    final summary = Map<String, dynamic>.from(connectionData['summary'] as Map? ?? const {});
    return RefreshIndicator(
      onRefresh: load,
      child: ListView(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(12, 0, 12, 24),
        children: [
          Padding(padding: const EdgeInsets.fromLTRB(4, 0, 4, 8), child: Text(context.t('analytics.scanned', args: {'count': connectionData['scanned'] ?? 0}))),
          _summaryList(context.t('analytics.userUsage'), List<dynamic>.from(summary['users'] as List? ?? const [])),
          _summaryList(context.t('analytics.inbounds'), List<dynamic>.from(summary['inbounds'] as List? ?? const [])),
          _summaryList(context.t('analytics.outbounds'), List<dynamic>.from(summary['outbounds'] as List? ?? const [])),
          _summaryList(context.t('analytics.nodes'), List<dynamic>.from(summary['endpoints'] as List? ?? const [])),
          _summaryList(context.t('analytics.destinations'), List<dynamic>.from(summary['destinations'] as List? ?? const [])),
          if (items.isEmpty)
            EmptyState(label: context.t('analytics.noConnections'))
          else
            for (final raw in items.take(100)) _connectionTile(Map<String, dynamic>.from(raw as Map), openRaw: true),
        ],
      ),
    );
  }

  Widget _summaryList(String title, List<dynamic> values) => Card(
        child: ExpansionTile(
          initiallyExpanded: values.isNotEmpty,
          title: Text(title, style: const TextStyle(fontWeight: FontWeight.w700)),
          children: values.isEmpty
              ? [ListTile(title: Text(context.t('analytics.noConnections')))]
              : [
                  for (final raw in values.take(20))
                    Builder(builder: (context) {
                      final item = Map<String, dynamic>.from(raw as Map);
                      return ListTile(
                        dense: true,
                        title: Text(item['tag']?.toString() ?? '—'),
                        subtitle: Text('${context.t('analytics.lastSeen')} ${formatTimestamp(item['lastSeen'])}'),
                        trailing: Text(item['count']?.toString() ?? '0'),
                        onTap: () => _showConnectionDetails(item['resource']?.toString() ?? 'all', item['tag']?.toString() ?? ''),
                      );
                    }),
                ],
        ),
      );

  Widget _connectionTile(Map<String, dynamic> item, {bool openRaw = false}) {
    final sourceInfo = _endpointInfo(item, 'sourceInfo');
    final destinationInfo = _endpointInfo(item, 'destinationInfo');
    final meta = _endpointSummary(context, destinationInfo) ?? _endpointSummary(context, sourceInfo);
    return ListTile(
      dense: true,
      leading: Icon(item['resource'] == 'outbound' ? Icons.call_made : Icons.call_received),
      title: Text('${item['resource']}/${item['protocol']}[${item['tag']}]'),
      subtitle: Text(
        [
          '${item['time'] ?? formatTimestamp(item['timestamp'])} · ${item['user']?.toString().isNotEmpty == true ? item['user'] : context.t('common.all')} · ${_connectionTarget(item)}',
          if (meta != null) meta,
        ].join('\n'),
        maxLines: 3,
        overflow: TextOverflow.ellipsis,
      ),
      trailing: const Icon(Icons.chevron_right),
      onTap: openRaw ? () => _showConnectionLog(item) : () => _showConnectionDetails(item['resource']?.toString() ?? 'all', item['tag']?.toString() ?? ''),
    );
  }

  Widget _logs() {
    if (logItems.isEmpty) return EmptyState(label: context.t('logs.empty'));
    return RefreshIndicator(
      onRefresh: load,
      child: ListView.builder(
        physics: const AlwaysScrollableScrollPhysics(),
        padding: const EdgeInsets.fromLTRB(12, 4, 12, 24),
        itemCount: logItems.length + 1,
        itemBuilder: (context, index) {
          if (index == 0) return Padding(padding: const EdgeInsets.fromLTRB(4, 0, 4, 8), child: Text(context.t('logs.count', args: {'count': logTotal})));
          return _logCard(Map<String, dynamic>.from(logItems[index - 1] as Map));
        },
      ),
    );
  }

  Widget _logCard(Map<String, dynamic> item) {
    final logLevel = item['level']?.toString() ?? 'INFO';
    final color = _levelColor(logLevel);
    final connection = _connectionFromLog(item);
    final lookupAddress = connectionLookupAddress(connection);
    final connectionMeta = connection == null
        ? null
        : _endpointSummary(context, _endpointInfo(connection, 'destinationInfo')) ?? _endpointSummary(context, _endpointInfo(connection, 'sourceInfo'));
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: ExpansionTile(
        leading: CircleAvatar(radius: 18, backgroundColor: color.withValues(alpha: .15), child: Icon(Icons.receipt_long_outlined, size: 18, color: color)),
        title: Text(item['message']?.toString() ?? '', maxLines: 2, overflow: TextOverflow.ellipsis, style: const TextStyle(fontFamily: 'monospace', fontSize: 13)),
        subtitle: Text(
          [
            '${item['time'] ?? formatTimestamp(item['timestamp'])} · ${item['source'] ?? 'system'}${item['user']?.toString().isNotEmpty == true ? ' · ${item['user']}' : ''}',
            if (connectionMeta != null) connectionMeta,
          ].join('\n'),
          maxLines: 3,
          overflow: TextOverflow.ellipsis,
        ),
        trailing: Chip(label: Text(logLevel), side: BorderSide.none),
        onExpansionChanged: (expanded) {
          if (expanded) _resolveLogConnection(item);
        },
        children: [
          if (lookupAddress != null && resolvingAddresses.contains(lookupAddress)) const LinearProgressIndicator(minHeight: 2),
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 16),
            child: Align(
              alignment: Alignment.centerLeft,
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  if (connection != null) ..._endpointDetailWidgets(context, connection),
                  SelectableText(item['message']?.toString() ?? '', style: const TextStyle(fontFamily: 'monospace')),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  Future<Map<String, dynamic>> _resolveConnectionInfo(Map<String, dynamic> item) async {
    final result = Map<String, dynamic>.from(item);
    final address = connectionLookupAddress(result);
    final infoKey = connectionInfoKey(result);
    if (address == null || infoKey == null) return result;
    final current = result[infoKey];
    if (current is Map && connectionInfoComplete(current)) return result;
    try {
      final info = Map<String, dynamic>.from(await context.read<AppState>().api!.get('analytics/address-info', query: {'address': address}) as Map);
      applyResolvedConnectionInfo(result, info);
    } catch (_) {
      // The raw connection remains available if enrichment cannot be reached.
    }
    return result;
  }

  Future<void> _resolveLogConnection(Map<String, dynamic> item) async {
    final rawConnection = item['connection'];
    if (rawConnection is! Map) return;
    final address = connectionLookupAddress(rawConnection);
    if (address == null || resolvingAddresses.contains(address) || resolvedAddresses.contains(address)) return;
    final infoKey = connectionInfoKey(rawConnection);
    if (infoKey == null) return;
    final current = rawConnection[infoKey];
    if (current is Map && connectionInfoComplete(current)) {
      resolvedAddresses.add(address);
      return;
    }

    setState(() => resolvingAddresses.add(address));
    final resolved = await _resolveConnectionInfo(Map<String, dynamic>.from(rawConnection));
    final resolvedInfo = resolved[infoKey];
    if (resolvedInfo is Map) {
      final info = Map<String, dynamic>.from(resolvedInfo);
      applyResolvedConnectionInfo(rawConnection, info);
      if (connectionInfoComplete(info)) resolvedAddresses.add(address);
    }
    if (mounted) setState(() => resolvingAddresses.remove(address));
  }

  Future<void> _showConnectionDetails(String resource, String tag) async {
    if (tag.isEmpty) return;
    try {
      final request = context.read<AppState>().api!.get('analytics/connections', query: {
        'resource': resource,
        'tag': tag,
        'search': search.text.trim(),
        'start': unixStartOfDay(start),
        'end': unixEndOfDay(end),
        'limit': 500,
      }).then((value) => Map<String, dynamic>.from(value as Map));
      if (!mounted) return;
      await showModalBottomSheet<void>(
        context: context,
        isScrollControlled: true,
        builder: (sheetContext) => DraggableScrollableSheet(
          expand: false,
          initialChildSize: .75,
          maxChildSize: .95,
          builder: (context, controller) => FutureBuilder<Map<String, dynamic>>(
            future: request,
            builder: (context, snapshot) {
              final items = List<dynamic>.from(snapshot.data?['items'] as List? ?? const []);
              return ListView(
                controller: controller,
                padding: const EdgeInsets.all(16),
                children: [
                  Text('${sheetContext.t('analytics.connectionDetails')} · $resource/$tag', style: Theme.of(sheetContext).textTheme.titleLarge),
                  const SizedBox(height: 12),
                  if (snapshot.connectionState != ConnectionState.done)
                    const Padding(padding: EdgeInsets.symmetric(vertical: 32), child: Center(child: CircularProgressIndicator()))
                  else if (snapshot.hasError)
                    EmptyState(label: snapshot.error.toString(), icon: Icons.error_outline)
                  else if (items.isEmpty)
                    EmptyState(label: sheetContext.t('analytics.noConnections'))
                  else
                    for (final raw in items) _connectionTile(Map<String, dynamic>.from(raw as Map), openRaw: true),
                ],
              );
            },
          ),
        ),
      );
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    }
  }

  Future<void> _showConnectionLog(Map<String, dynamic> item) async {
    final resolved = _resolveConnectionInfo(item);
    await showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      builder: (sheetContext) => SafeArea(
        child: FutureBuilder<Map<String, dynamic>>(
          future: resolved,
          initialData: item,
          builder: (context, snapshot) {
            final current = snapshot.data ?? item;
            return ConstrainedBox(
              constraints: BoxConstraints(maxHeight: MediaQuery.sizeOf(sheetContext).height * .9),
              child: SingleChildScrollView(
                padding: EdgeInsets.fromLTRB(16, 16, 16, 16 + MediaQuery.viewInsetsOf(sheetContext).bottom),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(sheetContext.t('analytics.connectionLog'), style: Theme.of(sheetContext).textTheme.titleLarge),
                    if (snapshot.connectionState != ConnectionState.done) const LinearProgressIndicator(minHeight: 2),
                    const SizedBox(height: 12),
                    _detailLine(sheetContext, sheetContext.t('logs.level'), current['level']?.toString()),
                    _detailLine(sheetContext, sheetContext.t('logs.time'), current['time']?.toString() ?? formatTimestamp(current['timestamp'])),
                    _detailLine(sheetContext, sheetContext.t('analytics.resource'), '${current['resource']}/${current['protocol']}[${current['tag']}]'),
                    _detailLine(sheetContext, sheetContext.t('analytics.user'), current['user']?.toString()),
                    _detailLine(sheetContext, sheetContext.t('analytics.destination'), current['destination']?.toString()),
                    _detailLine(sheetContext, sheetContext.t('analytics.source'), current['source']?.toString()),
                    ..._endpointDetailWidgets(sheetContext, current),
                    const Divider(height: 24),
                    Text(sheetContext.t('analytics.rawMessage'), style: Theme.of(sheetContext).textTheme.titleSmall),
                    const SizedBox(height: 8),
                    SelectableText(current['message']?.toString() ?? '', style: const TextStyle(fontFamily: 'monospace')),
                  ],
                ),
              ),
            );
          },
        ),
      ),
    );
  }

  Widget _detailLine(BuildContext context, String label, String? value) => Padding(
        padding: const EdgeInsets.only(bottom: 6),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SizedBox(width: 96, child: Text(label, style: Theme.of(context).textTheme.bodySmall?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant))),
            Expanded(child: SelectableText(value?.isNotEmpty == true ? value! : '—')),
          ],
        ),
      );
}

class _SummaryCard extends StatelessWidget {
  const _SummaryCard({required this.label, required this.value, required this.color});
  final String label;
  final String value;
  final Color color;

  @override
  Widget build(BuildContext context) => Card(
        child: Padding(
          padding: const EdgeInsets.all(12),
          child: Column(children: [Text(label, style: TextStyle(color: color)), const SizedBox(height: 5), FittedBox(child: Text(value, style: const TextStyle(fontWeight: FontWeight.w700)))]),
        ),
      );
}

class _TrafficPainter extends CustomPainter {
  _TrafficPainter(this.upload, this.download);
  final List<Map<String, dynamic>> upload;
  final List<Map<String, dynamic>> download;

  @override
  void paint(Canvas canvas, Size size) {
    final grid = Paint()..color = Colors.grey.withValues(alpha: .2)..strokeWidth = 1;
    for (var index = 0; index <= 4; index++) {
      final y = size.height * index / 4;
      canvas.drawLine(Offset(0, y), Offset(size.width, y), grid);
    }
    final all = [...upload, ...download];
    if (all.isEmpty) return;
    final minTime = all.map((item) => _int(item['dateTime'])).reduce((a, b) => a < b ? a : b);
    final maxTime = all.map((item) => _int(item['dateTime'])).reduce((a, b) => a > b ? a : b);
    final maxTraffic = math.max(1, all.map((item) => _int(item['traffic'])).reduce((a, b) => a > b ? a : b)).toInt();
    drawSeries(canvas, size, upload, Colors.orange, minTime, maxTime, maxTraffic);
    drawSeries(canvas, size, download, Colors.green, minTime, maxTime, maxTraffic);
  }

  void drawSeries(Canvas canvas, Size size, List<Map<String, dynamic>> values, Color color, int minTime, int maxTime, int maxTraffic) {
    if (values.isEmpty) return;
    final paint = Paint()..color = color..style = PaintingStyle.stroke..strokeWidth = 2;
    final pointPaint = Paint()..color = color..style = PaintingStyle.fill;
    final path = Path();
    final points = <Offset>[];
    for (var index = 0; index < values.length; index++) {
      final time = _int(values[index]['dateTime']);
      final traffic = _int(values[index]['traffic']);
      final x = maxTime == minTime ? size.width / 2 : (time - minTime) / (maxTime - minTime) * size.width;
      final y = size.height - (traffic / maxTraffic * size.height);
      points.add(Offset(x, y));
      if (index == 0) {
        path.moveTo(x, y);
      } else {
        path.lineTo(x, y);
      }
    }
    canvas.drawPath(path, paint);
    for (final point in points) {
      canvas.drawCircle(point, 3.25, pointPaint);
      canvas.drawCircle(point, 1.25, Paint()..color = Colors.white);
    }
  }

  @override
  bool shouldRepaint(covariant _TrafficPainter oldDelegate) => oldDelegate.upload != upload || oldDelegate.download != download;
}

int _int(dynamic value) => int.tryParse(value?.toString() ?? '') ?? 0;
String _date(DateTime value) => '${value.year}-${value.month.toString().padLeft(2, '0')}-${value.day.toString().padLeft(2, '0')}';
String _connectionTarget(Map<String, dynamic> item) =>
    item['destination']?.toString().isNotEmpty == true ? item['destination'].toString() : item['source']?.toString().isNotEmpty == true ? item['source'].toString() : '—';
Color _levelColor(String level) => switch (level) {
      'ERROR' => Colors.red,
      'WARNING' => Colors.orange,
      'DEBUG' => Colors.grey,
      _ => Colors.blue,
    };

Map<String, dynamic>? _connectionFromLog(Map<String, dynamic> item) {
  final connection = item['connection'];
  if (connection is Map) return Map<String, dynamic>.from(connection);
  return null;
}

Map<String, dynamic>? _endpointInfo(Map<String, dynamic> item, String key) {
  final raw = item[key];
  if (raw is Map) return Map<String, dynamic>.from(raw);
  return null;
}

String? _endpointSummary(BuildContext context, Map<String, dynamic>? info) {
  if (info == null) return null;
  final summary = connectionEndpointSummary(info, fallback: _scopeLabel(context, info['scope']?.toString()));
  return summary.isEmpty ? null : summary;
}

List<Widget> _endpointDetailWidgets(BuildContext context, Map<String, dynamic> item) {
  final source = _endpointInfo(item, 'sourceInfo');
  final destination = _endpointInfo(item, 'destinationInfo');
  return [
    if (destination != null) ..._endpointLines(context, context.t('analytics.destination'), destination),
    if (source != null) ..._endpointLines(context, context.t('analytics.source'), source),
  ];
}

List<Widget> _endpointLines(BuildContext context, String title, Map<String, dynamic> info) {
  final target = connectionEndpointTarget(info);
  final ownership = connectionEndpointOwnership(info, fallback: _scopeLabel(context, info['scope']?.toString()));
  final values = <MapEntry<String, String>>[
    MapEntry(context.t('analytics.ipAddress'), target),
    MapEntry(context.t('analytics.ipAttribution'), ownership),
  ].where((entry) => entry.value.isNotEmpty).toList();
  if (values.isEmpty) return const <Widget>[];
  return [
    Padding(
      padding: const EdgeInsets.only(top: 8, bottom: 4),
      child: Text(title, style: Theme.of(context).textTheme.titleSmall),
    ),
    for (final value in values)
      Padding(
        padding: const EdgeInsets.only(bottom: 4),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SizedBox(width: 96, child: Text(value.key, style: Theme.of(context).textTheme.bodySmall?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant))),
            Expanded(child: SelectableText(value.value)),
          ],
        ),
      ),
  ];
}

String _scopeLabel(BuildContext context, String? scope) {
  if (scope == null || scope.isEmpty) return '';
  final key = switch (scope) {
    'private' => 'analytics.scopePrivate',
    'loopback' => 'analytics.scopeLoopback',
    'link_local' => 'analytics.scopeLinkLocal',
    'multicast' => 'analytics.scopeMulticast',
    'reserved' => 'analytics.scopeReserved',
    'unspecified' => 'analytics.scopeReserved',
    'public' => 'analytics.scopePublic',
    'domain' => 'analytics.scopeDomain',
    _ => 'analytics.scopeUnknown',
  };
  return context.t(key);
}
