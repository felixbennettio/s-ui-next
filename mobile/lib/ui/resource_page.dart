import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';
import 'package:qr_flutter/qr_flutter.dart';

import '../core/app_locale_context.dart';
import '../state/app_state.dart';
import 'visual_editor.dart';
import 'widgets.dart';

class ResourcePage extends StatefulWidget {
  const ResourcePage({super.key, required this.resource, required this.title, required this.icon});

  final String resource;
  final String title;
  final IconData icon;

  @override
  State<ResourcePage> createState() => _ResourcePageState();
}

class _ResourcePageState extends State<ResourcePage> {
  final search = TextEditingController();
  List<dynamic> items = [];
  bool loading = true;
  String? error;

  @override
  void initState() {
    super.initState();
    load();
  }

  @override
  void dispose() {
    search.dispose();
    super.dispose();
  }

  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
    });
    try {
      final result = await context.read<AppState>().getResource(widget.resource);
      final value = result is List ? result : const [];
      if (mounted) setState(() => items = List<dynamic>.from(value));
    } catch (exception) {
      if (mounted) setState(() => error = exception.toString());
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  List<dynamic> get filtered {
    final query = search.text.trim().toLowerCase();
    if (query.isEmpty) return items;
    return items.where((item) => jsonEncode(item).toLowerCase().contains(query)).toList();
  }

  dynamic template() => VisualEditorSchema.forResource(widget.resource).defaultValue();

  Future<void> edit(dynamic item, String action) async {
    await showDialog<bool>(
      context: context,
      builder: (_) => VisualEditorDialog(
        title: '${widget.title} · ${_actionName(action)}',
        resource: widget.resource,
        initialValue: item,
        onSave: (value) async {
          final result = await context.read<AppState>().saveResource(widget.resource, action, value, apply: true);
          if (mounted) showSaveResult(context, result);
        },
        onSaveOnly: widget.resource == 'endpoints'
            ? (value) async {
                final result = await context.read<AppState>().saveResource(widget.resource, action, value, apply: false);
                if (mounted) showSaveResult(context, result);
              }
            : null,
      ),
    );
    if (mounted) await load();
  }

  Future<void> remove(dynamic item) async {
    if (!await confirm(context, title: context.tr('resource.deleteTitle', args: {'title': widget.title}), message: context.tr('resource.deleteMessage'), action: context.tr('common.delete'))) return;
    if (!mounted) return;
    try {
      final value = item is Map
          ? (widget.resource == 'clients' || widget.resource == 'tls' ? item['id'] : item['tag'])
          : item;
      final result = await context.read<AppState>().saveResource(widget.resource, 'del', value);
      await load();
      if (mounted) showSaveResult(context, result);
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    }
  }

  Future<void> bulk() async {
    var action = 'addbulk';
    var executing = false;
    final controller = TextEditingController(text: '[]');
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          title: Text(context.t('resource.bulk')),
          content: SizedBox(
            width: 620,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                AnchoredSelect<String>(
                  value: action,
                  options: [
                    SelectOption('addbulk', context.t('resource.bulkAdd')),
                    SelectOption('editbulk', context.t('resource.bulkEdit')),
                    SelectOption('delbulk', context.t('resource.bulkDelete')),
                  ],
                  onChanged: (value) => setDialogState(() => action = value),
                  label: context.t('resource.action'),
                ),
                const SizedBox(height: 12),
                TextField(controller: controller, minLines: 8, maxLines: 16, style: const TextStyle(fontFamily: 'monospace'), decoration: InputDecoration(labelText: context.t('resource.jsonArray'))),
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: executing ? null : () => Navigator.pop(dialogContext), child: Text(context.t('common.cancel'))),
            FilledButton(
              onPressed: executing ? null : () async {
                setDialogState(() => executing = true);
                try {
                  final value = jsonDecode(controller.text);
                  final result = await this.context.read<AppState>().saveResource(widget.resource, action, value);
                  if (mounted) showSaveResult(this.context, result);
                  if (dialogContext.mounted) Navigator.pop(dialogContext);
                  await load();
                } catch (exception) {
                  if (dialogContext.mounted) showMessage(dialogContext, exception.toString(), error: true);
                } finally {
                  if (dialogContext.mounted) setDialogState(() => executing = false);
                }
              },
              child: Text(context.t('resource.execute')),
            ),
          ],
        ),
      ),
    );
    controller.dispose();
  }

  Future<void> checkOutbound(Map<String, dynamic> item) async {
    try {
      final result = Map<String, dynamic>.from(await context.read<AppState>().api!.get('tools/check-outbound', query: {'tag': item['tag']}) as Map);
      if (!mounted) return;
      final ok = result['OK'] == true;
      final delay = result['Delay'];
      showMessage(context, ok ? context.tr('resource.checkOk', args: {'delay': delay == null ? '' : ' · ${delay}ms'}) : result['Error']?.toString() ?? context.tr('resource.checkFailed'), error: !ok);
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    }
  }

  Future<void> showClientQr(Map<String, dynamic> summary) async {
    try {
      final result = await context.read<AppState>().getResource('clients', id: summary['id']?.toString());
      final list = result is List ? result : const [];
      if (!mounted) return;
      if (list.isEmpty) throw FormatException(context.tr('resource.userNotFound'));
      final client = Map<String, dynamic>.from(list.first as Map);
      final state = context.read<AppState>();
      final panel = state.bootstrap['panel'];
      final subBase = panel is Map ? panel['subURI']?.toString() ?? '' : '';
      final values = <_QrValue>[];
      if (subBase.isNotEmpty) {
        final subId = client['subId']?.toString().trim().isNotEmpty == true ? client['subId'].toString() : client['name'].toString();
        final subscription = '$subBase${Uri.encodeComponent(subId)}';
        values.addAll([
          _QrValue(context.tr('resource.subscription'), subscription),
          _QrValue(context.tr('resource.jsonSubscription'), '$subscription?format=json'),
          _QrValue(context.tr('resource.clashSubscription'), '$subscription?format=clash'),
          _QrValue(context.tr('resource.singboxImport'), 'sing-box://import-remote-profile?url=${Uri.encodeComponent('$subscription?format=json')}#${client['name']}'),
        ]);
      }
      final links = client['links'];
      if (links is List) {
        for (final raw in links) {
          if (raw is Map && raw['uri'] != null) values.add(_QrValue(raw['remark']?.toString() ?? raw['type']?.toString() ?? context.tr('resource.shareLink'), raw['uri'].toString()));
        }
      }
      if (!mounted) return;
      await _showQrValues('${client['name']} · ${context.tr('resource.qrcode')}', values);
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    }
  }

  Future<void> showWireguardQr(Map<String, dynamic> item) async {
    final peers = item['peers'];
    if (peers is! List) {
      showMessage(context, context.tr('resource.noWireguardPeers'), error: true);
      return;
    }
    try {
      final api = context.read<AppState>().api!;
      final fallbackNames = [
        for (var index = 0; index < peers.length; index++) context.tr('resource.wireguardPeer', args: {'index': index + 1}),
      ];
      final values = <_QrValue>[];
      for (var index = 0; index < peers.length; index++) {
        final peer = peers[index];
        if (peer is! Map || !_isExportableWireGuardPeer(item, peer)) continue;
        final result = Map<String, dynamic>.from(
          await api.post('wireguard/export', data: {'tag': item['tag'], 'peerIndex': index}) as Map,
        );
        values.add(_QrValue(result['name']?.toString() ?? fallbackNames[index], result['config']?.toString() ?? ''));
      }
      if (!mounted) return;
      await _showQrValues('${item['tag']} · WireGuard', values);
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    }
  }

  bool _hasExportableWireGuardPeer(Map<String, dynamic> item) {
    final exportEnabled = item.containsKey('client_export_enabled')
        ? boolValue(item['client_export_enabled'])
        : (item['advertised_endpoint_host']?.toString().isNotEmpty ?? false);
    if (!exportEnabled) return false;
    final peers = item['peers'];
    return peers is List && peers.any((peer) => peer is Map && _isExportableWireGuardPeer(item, peer));
  }

  bool _isExportableWireGuardPeer(Map<String, dynamic> item, Map<dynamic, dynamic> peer) {
    if (peer['peer_key_mode'] == 'existing_peer') return false;
    if ((peer['client_private_key']?.toString().isNotEmpty ?? false) || boolValue(peer['client_private_key_set'])) return true;
    final keys = item['ext'] is Map ? (item['ext'] as Map)['keys'] : null;
    return keys is List && keys.any((key) => key is Map && key['public_key'] == peer['public_key']);
  }

  Future<void> _showQrValues(String title, List<_QrValue> values) async {
    if (values.isEmpty) {
      showMessage(context, context.tr('resource.noLinks'), error: true);
      return;
    }
    await showDialog<void>(
      context: context,
      builder: (dialogContext) => Dialog.fullscreen(
        child: Scaffold(
          appBar: AppBar(title: Text(title), leading: IconButton(onPressed: () => Navigator.pop(dialogContext), icon: const Icon(Icons.close))),
          body: ListView(
            padding: const EdgeInsets.all(20),
            children: [
              for (final value in values)
                Card(
                  margin: const EdgeInsets.only(bottom: 16),
                  child: Padding(
                    padding: const EdgeInsets.all(16),
                    child: Column(
                      children: [
                        Text(value.label, style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700)),
                        const SizedBox(height: 12),
                        LayoutBuilder(builder: (context, constraints) => ColoredBox(color: Colors.white, child: Padding(padding: const EdgeInsets.all(10), child: QrImageView(data: value.value, size: (constraints.maxWidth - 20).clamp(0, 260).toDouble())))),
                        const SizedBox(height: 10),
                        SelectableText(value.value, maxLines: 4),
                        TextButton.icon(onPressed: () => copyText(context, value.value), icon: const Icon(Icons.copy), label: Text(context.t('resource.copy'))),
                      ],
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        PageHeader(
          title: widget.title,
          subtitle: context.t('resource.subtitle'),
          actions: [
            IconButton.filledTonal(tooltip: context.t('resource.bulk'), onPressed: bulk, icon: const Icon(Icons.playlist_add)),
            const SizedBox(width: 8),
            IconButton.filled(tooltip: context.t('resource.new'), onPressed: () => edit(template(), 'new'), icon: const Icon(Icons.add)),
          ],
        ),
        Padding(
          padding: const EdgeInsets.fromLTRB(12, 4, 12, 10),
          child: TextField(
            controller: search,
            onChanged: (_) => setState(() {}),
            decoration: InputDecoration(labelText: context.t('resource.search', args: {'title': widget.title}), prefixIcon: const Icon(Icons.search), suffixIcon: search.text.isEmpty ? null : IconButton(onPressed: () => setState(search.clear), icon: const Icon(Icons.clear))),
          ),
        ),
        Expanded(
          child: loading
              ? const Center(child: CircularProgressIndicator())
              : error != null
                  ? EmptyState(label: error!, icon: Icons.error_outline)
                  : filtered.isEmpty
                      ? EmptyState(label: context.t('resource.empty'))
                      : RefreshIndicator(
                          onRefresh: load,
                          child: ListView.builder(
                            padding: const EdgeInsets.fromLTRB(12, 0, 12, 24),
                            itemCount: filtered.length,
                            itemBuilder: (context, index) => _resourceCard(filtered[index]),
                          ),
                        ),
        ),
      ],
    );
  }

  Widget _resourceCard(dynamic raw) {
    final item = raw is Map ? Map<String, dynamic>.from(raw) : <String, dynamic>{'value': raw};
    final title = item['name']?.toString().isNotEmpty == true ? item['name'].toString() : item['tag']?.toString().isNotEmpty == true ? item['tag'].toString() : '#${item['id'] ?? '—'}';
    final subtitle = [item['type'], if (item['group']?.toString().isNotEmpty == true) item['group'], if (item['listen_port'] != null) ':${item['listen_port']}'].where((value) => value != null).join(' · ');
    final enabled = item['enable'];
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      child: ListTile(
        contentPadding: const EdgeInsets.fromLTRB(16, 8, 8, 8),
        leading: CircleAvatar(
          backgroundColor: enabled == false ? Theme.of(context).colorScheme.errorContainer : Theme.of(context).colorScheme.primaryContainer,
          child: Icon(widget.icon, color: enabled == false ? Theme.of(context).colorScheme.onErrorContainer : Theme.of(context).colorScheme.onPrimaryContainer),
        ),
        title: Text(title, style: const TextStyle(fontWeight: FontWeight.w700)),
        subtitle: subtitle.isEmpty ? null : Text(subtitle),
        trailing: PopupMenuButton<String>(
          onSelected: (action) {
            switch (action) {
              case 'edit':
                edit(item, 'edit');
                return;
              case 'clone':
                final clone = Map<String, dynamic>.from(item)..['id'] = 0;
                edit(clone, 'new');
                return;
              case 'copy':
                copyText(context, prettyJson(item));
                return;
              case 'delete':
                remove(item);
                return;
              case 'test':
                checkOutbound(item);
                return;
              case 'qr-client':
                showClientQr(item);
                return;
              case 'qr-wireguard':
                showWireguardQr(item);
                return;
            }
          },
          itemBuilder: (_) => [
            PopupMenuItem(value: 'edit', child: ListTile(leading: const Icon(Icons.edit_outlined), title: Text(context.t('resource.edit')))),
            PopupMenuItem(value: 'clone', child: ListTile(leading: const Icon(Icons.copy_all_outlined), title: Text(context.t('resource.clone')))),
            if (widget.resource == 'outbounds') PopupMenuItem(value: 'test', child: ListTile(leading: const Icon(Icons.speed_outlined), title: Text(context.t('resource.connectionTest')))),
            if (widget.resource == 'clients') PopupMenuItem(value: 'qr-client', child: ListTile(leading: const Icon(Icons.qr_code), title: Text(context.t('resource.subscriptionQr')))),
            if (widget.resource == 'endpoints' && item['type'] == 'wireguard' && _hasExportableWireGuardPeer(item)) PopupMenuItem(value: 'qr-wireguard', child: ListTile(leading: const Icon(Icons.qr_code), title: Text(context.t('resource.wireguardQr')))),
            PopupMenuItem(value: 'copy', child: ListTile(leading: const Icon(Icons.content_copy), title: Text(context.t('resource.copyJson')))),
            PopupMenuItem(value: 'delete', child: ListTile(leading: const Icon(Icons.delete_outline), title: Text(context.t('common.delete')))),
          ],
        ),
        onTap: () => edit(item, 'edit'),
      ),
    );
  }

  String _actionName(String action) => {'new': context.t('resource.new'), 'edit': context.t('resource.edit')}[action] ?? action;
}

class _QrValue {
  const _QrValue(this.label, this.value);
  final String label;
  final String value;
}
