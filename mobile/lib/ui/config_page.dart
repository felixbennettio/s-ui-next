import 'dart:convert';

import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../core/app_locale_context.dart';
import '../state/app_state.dart';
import 'visual_editor.dart';
import 'widgets.dart';

class ConfigPage extends StatefulWidget {
  const ConfigPage({super.key, this.settingsOnly = false});

  final bool settingsOnly;

  @override
  State<ConfigPage> createState() => _ConfigPageState();
}

class _ConfigPageState extends State<ConfigPage> with SingleTickerProviderStateMixin {
  late final TabController tabs;
  Map<String, dynamic> config = {};
  Map<String, dynamic> settings = {};
  bool loading = true;
  String? error;

  @override
  void initState() {
    super.initState();
    tabs = TabController(length: 4, vsync: this);
    load();
  }

  @override
  void dispose() {
    tabs.dispose();
    super.dispose();
  }

  Future<void> load() async {
    setState(() {
      loading = true;
      error = null;
    });
    try {
      final state = context.read<AppState>();
      final result = await state.getResource(widget.settingsOnly ? 'settings' : 'config');
      if (mounted) {
        setState(() {
          if (widget.settingsOnly) {
            settings = Map<String, dynamic>.from(result as Map);
          } else {
            config = Map<String, dynamic>.from(result as Map);
          }
        });
      }
    } catch (exception) {
      if (mounted) setState(() => error = exception.toString());
    } finally {
      if (mounted) setState(() => loading = false);
    }
  }

  Future<void> editConfigSection(String title, List<String> keys, {bool full = false}) async {
    final section = <String, dynamic>{for (final key in keys) key: config[key]};
    await showDialog<bool>(
      context: context,
      builder: (_) => VisualEditorDialog(
        title: title,
        resource: 'config',
        initialJson: full,
        initialValue: section,
        onSave: (value) async {
          if (value is! Map) throw FormatException(context.tr('config.configObjectRequired'));
          final next = Map<String, dynamic>.from(full ? value : config);
          for (final key in full ? <String>[] : keys) {
            if (value.containsKey(key)) {
              next[key] = value[key];
            } else {
              next.remove(key);
            }
          }
          final result = await context.read<AppState>().saveResource('config', 'set', next);
          if (mounted) showSaveResult(context, result);
        },
      ),
    );
    if (mounted) await load();
  }

  Future<void> editSettings() async {
    await showDialog<bool>(
      context: context,
      builder: (_) => VisualEditorDialog(
        title: context.tr('config.panelSubscriptionSettings'),
        resource: 'settings',
        initialValue: settings,
        onSave: (value) async {
          if (value is! Map) throw FormatException(context.tr('config.settingsObjectRequired'));
          final result = await context.read<AppState>().saveResource('settings', 'set', value);
          if (mounted) showSaveResult(context, result);
        },
      ),
    );
    if (mounted) await load();
  }

  @override
  Widget build(BuildContext context) {
    if (loading) return const Center(child: CircularProgressIndicator());
    if (error != null) return EmptyState(label: error!, icon: Icons.error_outline);
    if (widget.settingsOnly) return _settingsSection();
    return Column(
      children: [
        PageHeader(title: context.t('config.title'), subtitle: context.t('config.subtitle')),
        TabBar(
          controller: tabs,
          isScrollable: true,
          tabs: [
            Tab(text: context.t('config.routing')),
            const Tab(text: 'DNS'),
            Tab(text: context.t('config.basics')),
            Tab(text: context.t('config.rawJson')),
          ],
        ),
        Expanded(
          child: TabBarView(
            controller: tabs,
            children: [
              _section(context.t('config.routingRulesets'), ['route'], Icons.route_outlined),
              _section('DNS', ['dns'], Icons.dns_outlined),
              _section(context.t('config.basicInfo'), ['log', 'ntp', 'experimental'], Icons.settings_input_component_outlined),
              _section(context.t('config.rawJson'), config.keys.toList(), Icons.data_object, full: true),
            ],
          ),
        ),
      ],
    );
  }

  Widget _section(String title, List<String> keys, IconData icon, {bool full = false}) {
    final value = <String, dynamic>{for (final key in keys) key: config[key]};
    return ListView(
      padding: const EdgeInsets.all(12),
      children: [
        Card(
          child: Padding(
            padding: const EdgeInsets.all(16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(children: [Icon(icon), const SizedBox(width: 10), Expanded(child: Text(title, style: Theme.of(context).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700))), FilledButton.tonalIcon(onPressed: () => editConfigSection(title, keys, full: full), icon: const Icon(Icons.edit_outlined), label: Text(context.t('config.edit')))]),
                const SizedBox(height: 16),
                SelectableText(const JsonEncoder.withIndent('  ').convert(value), style: const TextStyle(fontFamily: 'monospace', fontSize: 12)),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _settingsSection() {
    final groups = <String, List<String>>{
      context.t('config.panelInterface'): ['webListen', 'webPort', 'webPath', 'webDomain', 'webCertFile', 'webKeyFile', 'webURI', 'sessionMaxAge', 'trafficAge', 'timeLocation'],
      context.t('config.subscriptionService'): ['subListen', 'subPort', 'subPath', 'subDomain', 'subCertFile', 'subKeyFile', 'subUpdates', 'subEncode', 'subShowInfo', 'subInfoUpload', 'subInfoDownload', 'subInfoTotal', 'subInfoExpire', 'subInfoRemaining', 'subURI'],
      context.t('config.subscriptionExtensions'): ['subJsonExt', 'subClashExt'],
	  context.t('config.loginIdentity'): ['oidcEnabled', 'oidcIssuer', 'oidcClientId', 'oidcClientSecret', 'oidcRedirectUrl', 'oidcScopes', 'oidcUsernameClaim', 'oidcAllowedUsers', 'passkeyEnabled', 'passkeyRpId', 'passkeyOrigins'],
    };
    return ListView(
      padding: const EdgeInsets.all(12),
      children: [
        Align(alignment: Alignment.centerRight, child: FilledButton.icon(onPressed: editSettings, icon: const Icon(Icons.edit_outlined), label: Text(context.t('config.editAllSettings')))),
        const SizedBox(height: 8),
        for (final group in groups.entries)
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(group.key, style: Theme.of(context).textTheme.titleMedium?.copyWith(fontWeight: FontWeight.w700)),
                  const Divider(),
                  for (final key in group.value)
                    Padding(
                      padding: const EdgeInsets.symmetric(vertical: 5),
                      child: Row(crossAxisAlignment: CrossAxisAlignment.start, children: [SizedBox(width: 130, child: Text(key, style: TextStyle(color: Theme.of(context).colorScheme.onSurfaceVariant))), Expanded(child: SelectableText(settings[key]?.toString() ?? ''))]),
                    ),
                ],
              ),
            ),
          ),
      ],
    );
  }
}
