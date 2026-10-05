import 'package:flutter/material.dart';

import '../core/app_locale_context.dart';
import 'config_page.dart';
import 'tools_page.dart';

class SettingsPage extends StatelessWidget {
  const SettingsPage({super.key, this.initialTab = 0});
  final int initialTab;

  @override
  Widget build(BuildContext context) => DefaultTabController(
        length: 2,
        initialIndex: initialTab,
        child: Column(
          children: [
            TabBar(tabs: [
              Tab(text: context.t('config.panelSettings')),
              Tab(text: context.t('settings.tools')),
            ]),
            const Expanded(
              child: TabBarView(children: [
                ConfigPage(settingsOnly: true),
                ToolsPage(),
              ]),
            ),
          ],
        ),
      );
}
