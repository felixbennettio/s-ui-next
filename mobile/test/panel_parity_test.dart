import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:provider/provider.dart';
import 'package:sui_mobile/core/api_client.dart';
import 'package:sui_mobile/core/connection_profile.dart';
import 'package:sui_mobile/core/dashboard_sample.dart';
import 'package:sui_mobile/state/app_state.dart';
import 'package:sui_mobile/main.dart';
import 'package:sui_mobile/core/save_result.dart';
import 'package:sui_mobile/ui/config_page.dart';
import 'package:sui_mobile/ui/dashboard_page.dart';
import 'package:sui_mobile/ui/shell.dart';
import 'package:sui_mobile/ui/visual_editor.dart';

class _PanelApi extends ApiClient {
  _PanelApi() : super(const ConnectionProfile(name: 'test', baseUrl: 'https://example.test/'));
  int statusRequests = 0;
  int secretRequests = 0;
  bool failStatus = false;
  static const status = {
    'cpu': 24.5, 'sbd': {'running': true, 'stats': {'Uptime': 86400, 'Alloc': 67108864, 'NumGoroutine': 32}},
    'mem': {'current': 1073741824, 'total': 4294967296}, 'dsk': {'current': 10737418240, 'total': 42949672960},
    'swp': {'current': 0, 'total': 2147483648}, 'net': {'sent': 1000, 'recv': 2000},
    'db': {'clients': 12, 'inbounds': 4, 'outbounds': 3, 'endpoints': 2, 'services': 1, 'clientUp': 104857600, 'clientDown': 1073741824},
    'sys': {'hostName': 'S-UI Next', 'appVersion': '1.0.16', 'cpuType': 'ARM64', 'cpuCount': 4, 'bootTime': 1700000000},
  };

  @override
  Future<dynamic> get(String path, {Map<String, dynamic>? query}) async {
    if (path == 'status') {
      statusRequests++;
      if (failStatus) throw const ApiException('offline');
      return status;
    }
    if (path == 'onlines') return {'user': ['alice'], 'inbound': ['vless-entry'], 'outbound': ['direct']};
    if (path == 'logs') return {'items': [{'time': '12:00:00', 'level': 'INFO', 'message': 'sing-box started'}]};
    if (path == 'resources/clients') return [];
    if (path == 'resources/config') return {'route': {}, 'dns': {}, 'experimental': {}};
    if (path == 'resources/settings') return {};
    return {};
  }

  @override
  Future<dynamic> post(String path, {Object? data}) async {
    if (path == 'wireguard/secret') {
      secretRequests++;
      return 'test-only-secret';
    }
    return {};
  }
}

Widget _app(AppState state, Widget child) => ChangeNotifierProvider.value(value: state,
  child: MaterialApp(theme: SuiMobile.buildTheme(Brightness.light), home: Scaffold(body: RepaintBoundary(key: const ValueKey('preview'), child: child))));

Future<void> _preview(WidgetTester tester, String name) async {
  final directory = Platform.environment['SUI_PREVIEW_DIR'];
  if (directory != null) {
    await expectLater(find.byKey(const ValueKey('preview')), matchesGoldenFile('$directory/$name.png'));
  }
}

class _ConfigState extends AppState {
  Map<String, dynamic> config = {'route': {}, 'dns': {}};
  @override
  Future<dynamic> getResource(String resource, {String? id}) async => config;
  @override
  Future<SaveResult> saveResource(String resource, String action, dynamic data,
      {List<int> initUsers = const [], bool apply = true}) async {
    config = Map<String, dynamic>.from(data as Map);
    return const SaveResult();
  }
}

void main() {
  TestWidgetsFlutterBinding.ensureInitialized();
  setUpAll(() async {
    if (Platform.environment['SUI_PREVIEW_DIR'] == null) return;
    final root = Platform.environment['FLUTTER_ROOT'];
    final candidates = [
      '$root/bin/cache/artifacts/material_fonts/Roboto-Regular.ttf',
      '/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf',
    ];
    for (final path in candidates) {
      final font = File(path);
      if (!font.existsSync()) continue;
      await (FontLoader('Roboto')..addFont(font.readAsBytes().then(ByteData.sublistView))).load();
      break;
    }
  });

  testWidgets('full JSON config keeps new fields and removes deleted fields', (tester) async {
    final state = _ConfigState()..localeCode = 'en';
    await tester.pumpWidget(_app(state, const ConfigPage()));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Full JSON'));
    await tester.pumpAndSettle();
    await tester.tap(find.text('Edit'));
    await tester.pumpAndSettle();
    await tester.enterText(find.byType(TextField), '{"route":{},"experimental":{"cache_file":{"enabled":true}}}');
    await tester.tap(find.text('Save'));
    await tester.pumpAndSettle();
    expect(state.config.containsKey('dns'), isFalse);
    expect(state.config['experimental']['cache_file']['enabled'], isTrue);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
  });

  test('network rates use elapsed time and tolerate a reset counter', () {
    final time = DateTime(2026);
    final first = DashboardSample.fromStatus({'net': {'sent': 100, 'recv': 100}}, time, null);
    final next = DashboardSample.fromStatus({'net': {'sent': 1100, 'recv': 2100}}, time.add(const Duration(seconds: 2)), first);
    expect(next.uploadRate, 500);
    expect(next.downloadRate, 1000);
    final reset = DashboardSample.fromStatus({'net': {'sent': 0, 'recv': 0}}, time.add(const Duration(seconds: 4)), next);
    expect(reset.uploadRate, 0);
    expect(reset.downloadRate, 0);
  });

  for (final size in const [Size(320, 568), Size(390, 844), Size(1180, 820)]) {
    testWidgets('dashboard and navigation fit ${size.width}', (tester) async {
      tester.view.physicalSize = size;
      tester.view.devicePixelRatio = 1;
      addTearDown(tester.view.resetPhysicalSize);
      addTearDown(tester.view.resetDevicePixelRatio);
      final state = AppState()..api = _PanelApi()..localeCode = 'en';
      await tester.pumpWidget(_app(state, const AppShell()));
      await tester.pump();
      await tester.pump(const Duration(milliseconds: 300));
      expect(tester.takeException(), isNull);
      expect(find.text('Running'), findsOneWidget);
      await _preview(tester, 'dashboard-${size.width.toInt()}');
      if (size.width < 920) {
        await tester.tap(find.byTooltip('Open navigation menu'));
        await tester.pumpAndSettle();
      }
      expect(find.text('Overview'), findsOneWidget);
      expect(find.text('Access'), findsOneWidget);
      await tester.tap(find.widgetWithText(NavigationDrawerDestination, 'Users'));
      await tester.pumpAndSettle();
      expect(tester.takeException(), isNull);
      await tester.pumpWidget(const SizedBox());
    });
  }

  testWidgets('dashboard retains previous data on failure and stops polling in background', (tester) async {
    final api = _PanelApi();
    final state = AppState()..api = api..localeCode = 'en';
    await tester.pumpWidget(_app(state, const DashboardPage()));
    await tester.pump();
    api.failStatus = true;
    await tester.pump(const Duration(seconds: 2));
    await tester.pump();
    expect(find.textContaining('showing the last available data'), findsOneWidget);
    expect(find.text('Running'), findsOneWidget);
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.paused);
    final requests = api.statusRequests;
    await tester.pump(const Duration(seconds: 10));
    expect(api.statusRequests, requests);
    await tester.pumpWidget(const SizedBox());
    tester.binding.handleAppLifecycleStateChanged(AppLifecycleState.resumed);
  });

  testWidgets('stored key copies on demand without exposing it in the editor', (tester) async {
    final api = _PanelApi();
    final state = AppState()..api = api..localeCode = 'en';
    String? clipboard;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
      if (call.method == 'Clipboard.setData') clipboard = (call.arguments as Map)['text'] as String;
      return null;
    });
    addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
    final initial = {'id': 1, 'type': 'wireguard', 'private_key': '[redacted]', 'private_key_set': true};
    await tester.pumpWidget(_app(state, VisualEditorDialog(title: 'WireGuard', resource: 'endpoints',
      initialValue: initial, onSave: (_) async {})));
    expect(api.secretRequests, 0);
    await tester.tap(find.byTooltip('Copy').first);
    await tester.pumpAndSettle();
    expect(api.secretRequests, 1);
    expect(clipboard, 'test-only-secret');
    expect(initial['private_key'], '[redacted]');
    expect(find.text('test-only-secret'), findsNothing);
    expect(find.text('Copied'), findsOneWidget);
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('a pasted key copies the edited value and reports clipboard errors', (tester) async {
    final api = _PanelApi();
    final state = AppState()..api = api..localeCode = 'en';
    String? clipboard;
    var failClipboard = false;
    tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, (call) async {
      if (call.method == 'Clipboard.setData') {
        if (failClipboard) throw PlatformException(code: 'unavailable');
        clipboard = (call.arguments as Map)['text'] as String;
      }
      return null;
    });
    addTearDown(() => tester.binding.defaultBinaryMessenger.setMockMethodCallHandler(SystemChannels.platform, null));
    await tester.pumpWidget(_app(state, VisualEditorDialog(title: 'WireGuard', resource: 'endpoints',
      initialValue: const {'type': 'wireguard', 'private_key': ''}, onSave: (_) async {})));
    await tester.enterText(find.byType(TextFormField), 'new-test-key');
    await tester.pump();
    await tester.tap(find.byTooltip('Copy').first);
    await tester.pumpAndSettle();
    expect(clipboard, 'new-test-key');
    expect(api.secretRequests, 0);
    await tester.pump(const Duration(seconds: 5));
    failClipboard = true;
    await tester.tap(find.byTooltip('Copy').first);
    await tester.pumpAndSettle();
    expect(find.text('Copy failed. Please try again.'), findsOneWidget);
    expect(find.text('Copied'), findsNothing);
    await tester.pumpWidget(const SizedBox());
  });

  testWidgets('editor prevents duplicate saves and fits a narrow phone', (tester) async {
    tester.view.physicalSize = const Size(320, 568);
    tester.view.devicePixelRatio = 1;
    addTearDown(tester.view.resetPhysicalSize);
    addTearDown(tester.view.resetDevicePixelRatio);
    final state = AppState()..localeCode = 'en';
    final pending = Completer<void>();
    var saves = 0;
    await tester.pumpWidget(_app(state, VisualEditorDialog(title: 'WireGuard tunnel', resource: 'endpoints',
      initialValue: const {'type': 'wireguard', 'private_key': '', 'peers': []},
      onSave: (_) { saves++; return pending.future; }, onSaveOnly: (_) async {})));
    await tester.pump();
    expect(tester.takeException(), isNull);
    await _preview(tester, 'editor-320');
    await tester.tap(find.text('Save & apply'));
    await tester.pump();
    await tester.tap(find.text('Save & apply'));
    await tester.pump();
    expect(saves, 1);
    expect(tester.takeException(), isNull);
    await tester.pumpWidget(const SizedBox());
    pending.complete();
    await tester.pump();
  });
}
