import 'package:flutter/material.dart';
import 'package:provider/provider.dart';

import '../core/app_locale_context.dart';
import '../state/app_state.dart';
import 'admin_page.dart';
import 'analytics_page.dart';
import 'config_page.dart';
import 'dashboard_page.dart';
import 'resource_page.dart';
import 'settings_page.dart';
import 'widgets.dart';

class _Destination {
  const _Destination(this.groupKey, this.labelKey, this.icon, this.builder);
  final String groupKey;
  final String labelKey;
  final IconData icon;
  final Widget Function(BuildContext context) builder;
}

class AppShell extends StatefulWidget {
  const AppShell({super.key});

  @override
  State<AppShell> createState() => _AppShellState();
}

class _AppShellState extends State<AppShell> {
  int selected = 0;

  int refreshVersion = 0;
  bool openTools = false;

  void navigate(String key) {
    final tools = key == 'settings.tools';
    final index = destinations.indexWhere((item) => item.labelKey == (tools ? 'nav.tools' : key));
    if (index >= 0) setState(() {
      selected = index;
      openTools = tools;
    });
  }

  late final destinations = <_Destination>[
    _Destination('navigation.overview', 'nav.home', Icons.dashboard_outlined, (_) => DashboardPage(onNavigate: navigate)),
    _Destination('navigation.overview', 'nav.analytics', Icons.query_stats, (_) => const AnalyticsPage()),
    _Destination('navigation.access', 'nav.clients', Icons.people_outline, (context) => ResourcePage(resource: 'clients', title: context.t('nav.clients'), icon: Icons.people_outline)),
    _Destination('navigation.access', 'nav.inbounds', Icons.login, (context) => ResourcePage(resource: 'inbounds', title: context.t('nav.inbounds'), icon: Icons.login)),
    _Destination('navigation.access', 'nav.tls', Icons.workspace_premium_outlined, (context) => ResourcePage(resource: 'tls', title: context.t('nav.tls'), icon: Icons.workspace_premium_outlined)),
    _Destination('navigation.network', 'nav.outbounds', Icons.logout, (context) => ResourcePage(resource: 'outbounds', title: context.t('nav.outbounds'), icon: Icons.logout)),
    _Destination('navigation.network', 'nav.endpoints', Icons.vpn_key_outlined, (context) => ResourcePage(resource: 'endpoints', title: context.t('nav.endpoints'), icon: Icons.vpn_key_outlined)),
    _Destination('navigation.network', 'nav.config', Icons.route_outlined, (_) => const ConfigPage()),
    _Destination('navigation.operations', 'nav.services', Icons.dns_outlined, (context) => ResourcePage(resource: 'services', title: context.t('nav.services'), icon: Icons.dns_outlined)),
    _Destination('navigation.operations', 'nav.tools', Icons.settings_outlined, (_) => SettingsPage(initialTab: openTools ? 1 : 0)),
    _Destination('navigation.operations', 'nav.admin', Icons.admin_panel_settings_outlined, (_) => const AdminPage()),
  ];

  @override
  Widget build(BuildContext context) {
    final wide = MediaQuery.sizeOf(context).width >= 920;
    final state = context.watch<AppState>();
    final activePanelKey = state.profile?.id.isNotEmpty == true
        ? state.profile!.id
        : state.profile?.normalizedBaseUrl ?? '';
    final body = KeyedSubtree(
      key: ValueKey('$activePanelKey:$selected:$refreshVersion'),
      child: destinations[selected].builder(context),
    );

    return Scaffold(
      appBar: AppBar(
        title: Text(context.t(destinations[selected].labelKey)),
        centerTitle: !wide,
        actions: [
          IconButton(
            tooltip: context.t('common.refresh'),
            onPressed: state.busy
                ? null
                : () async {
                    try {
                      await state.refreshBootstrap();
                      if (mounted) setState(() => refreshVersion += 1);
                      if (context.mounted) showMessage(context, context.tr('common.refreshed'));
                    } catch (exception) {
                      if (context.mounted) showMessage(context, exception.toString(), error: true);
                    }
                  },
            icon: const Icon(Icons.refresh),
          ),
        ],
      ),
      drawer: wide ? null : _drawer(context, state),
      body: Row(
        children: [
          if (wide) SizedBox(width: 260, child: _drawer(context, state, persistent: true)),
          Expanded(child: body),
        ],
      ),
    );
  }

  Widget _drawer(BuildContext context, AppState state, {bool persistent = false}) {
    return NavigationDrawer(
      selectedIndex: selected,
      onDestinationSelected: (index) {
        setState(() { selected = index; openTools = false; });
        if (!persistent) Navigator.pop(context);
      },
      children: [
        Padding(
          padding: const EdgeInsets.fromLTRB(20, 24, 20, 12),
          child: Row(
            children: [
              const CircleAvatar(child: Icon(Icons.shield_outlined)),
              const SizedBox(width: 12),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'S-UI Next',
                      style: Theme.of(context).textTheme.titleLarge?.copyWith(
                            fontWeight: FontWeight.w700,
                          ),
                    ),
                    Text(state.profile?.name ?? '', overflow: TextOverflow.ellipsis),
                  ],
                ),
              ),
              IconButton(
                tooltip: context.t('nav.switchPanel'),
                onPressed: state.busy
                    ? null
                    : () {
                        if (!persistent) Navigator.pop(context);
                        if (mounted) _showPanelSwitcher(this.context);
                      },
                icon: const Icon(Icons.swap_horiz),
              ),
            ],
          ),
        ),
        const Divider(),
        for (var index = 0; index < destinations.length; index++) ...[
          if (index == 0 || destinations[index - 1].groupKey != destinations[index].groupKey)
            Padding(
              padding: const EdgeInsets.fromLTRB(28, 16, 16, 8),
              child: Text(context.t(destinations[index].groupKey), style: Theme.of(context).textTheme.labelMedium),
            ),
          NavigationDrawerDestination(
            icon: Icon(destinations[index].icon),
            label: Text(context.t(destinations[index].labelKey)),
          ),
        ],
        const Divider(),
        ListTile(
          leading: const Icon(Icons.logout),
          title: Text(context.t('nav.logout')),
          onTap: () async {
            if (!persistent) Navigator.pop(context);
            final revoke = await confirm(context, title: context.tr('nav.logoutTitle'), message: context.tr('nav.logoutMessage'), action: context.tr('nav.logoutRevoke'));
            await state.disconnect(revoke: revoke);
          },
        ),
      ],
    );
  }

  Future<void> _showPanelSwitcher(BuildContext context) {
    return showModalBottomSheet<void>(
      context: context,
      showDragHandle: true,
      builder: (sheetContext) => Consumer<AppState>(
        builder: (context, state, _) {
          final activeId = state.profile?.id;
          return SafeArea(
            child: ListView(
              shrinkWrap: true,
              children: [
                Padding(
                  padding: const EdgeInsets.fromLTRB(20, 4, 20, 12),
                  child: Text(
                    context.t('panelSwitcher.title'),
                    style: Theme.of(context).textTheme.titleLarge?.copyWith(fontWeight: FontWeight.w700),
                  ),
                ),
                for (final saved in state.profiles)
                  ListTile(
                    leading: Icon(saved.id == activeId ? Icons.check_circle : Icons.dns_outlined),
                    title: Text(saved.name, maxLines: 1, overflow: TextOverflow.ellipsis),
                    subtitle: Text(saved.normalizedBaseUrl, maxLines: 1, overflow: TextOverflow.ellipsis),
                    enabled: !state.busy && saved.id != activeId,
                    onTap: () async {
                      Navigator.pop(sheetContext);
                      try {
                        await state.switchProfile(saved);
                        if (context.mounted) showMessage(context, context.tr('panelSwitcher.connected', args: {'name': saved.name}));
                      } catch (exception) {
                        if (context.mounted) showMessage(context, exception.toString(), error: true);
                      }
                    },
                  ),
                const Divider(),
                ListTile(
                  leading: const Icon(Icons.add_link),
                  title: Text(context.t('panelSwitcher.add')),
                  onTap: () {
                    Navigator.pop(sheetContext);
                    state.prepareNewConnection();
                  },
                ),
                ListTile(
                  leading: const Icon(Icons.edit_outlined),
                  title: Text(context.t('panelSwitcher.reconfigure')),
                  onTap: () {
                    Navigator.pop(sheetContext);
                    state.reconfigure();
                  },
                ),
              ],
            ),
          );
        },
      ),
    );
  }
}
