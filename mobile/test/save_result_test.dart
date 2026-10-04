import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:sui_mobile/core/api_client.dart';
import 'package:sui_mobile/core/connection_profile.dart';
import 'package:sui_mobile/state/app_state.dart';

class _SaveApi extends ApiClient {
  _SaveApi() : super(const ConnectionProfile(name: 'test', baseUrl: 'https://example.test/'));
  int posts = 0;
  bool rejectSave = false;
  bool rejectRefresh = false;
  bool warning = false;
  Completer<dynamic>? pendingRefresh;

  @override
  Future<dynamic> post(String path, {Object? data}) async {
    posts++;
    if (rejectSave) throw const ApiException('save rejected');
    return {'resources': {'outbounds': [{'tag': 'saved'}]}, if (warning) 'warning': 'savedButRefreshFailed'};
  }

  @override
  Future<dynamic> get(String path, {Map<String, dynamic>? query}) async {
    if (pendingRefresh != null) return pendingRefresh!.future;
    if (rejectRefresh) throw const ApiException('refresh failed');
    return {'panel': {'outbounds': [{'tag': 'saved'}]}};
  }
}

void main() {
  test('committed save remains successful when bootstrap fails', () async {
    final api = _SaveApi()..rejectRefresh = true;
    final state = AppState()..api = api;
    final result = await state.saveResource('outbounds', 'new', {'tag': 'saved'});
    expect(result.refreshFailed, isTrue);
    expect(result.messageKey, 'common.savedRefreshFailed');
    expect(api.posts, 1);
    expect(state.bootstrap['panel']['outbounds'][0]['tag'], 'saved');
  });

  test('server refresh warning is preserved and save-only is explicit', () async {
    final api = _SaveApi()..warning = true;
    final state = AppState()..api = api;
    expect((await state.saveResource('outbounds', 'new', {})).refreshFailed, isTrue);
    api.warning = false;
    expect((await state.saveResource('endpoints', 'edit', {}, apply: false)).messageKey, 'common.saved');
    expect((await state.saveResource('config', 'set', {})).messageKey, 'common.savedApplied');
  });

  test('a rejected mutation still fails', () async {
    final api = _SaveApi()..rejectSave = true;
    final state = AppState()..api = api;
    await expectLater(state.saveResource('outbounds', 'new', {}), throwsA(isA<ApiException>()));
    expect(state.bootstrap, isEmpty);
  });

  test('late refresh cannot replace the newly selected panel', () async {
    final api = _SaveApi()..pendingRefresh = Completer<dynamic>();
    final state = AppState()..api = api;
    final request = state.refreshBootstrap(notify: false);
    state.api = _SaveApi();
    state.bootstrap = {'panel': {'name': 'new panel'}};
    api.pendingRefresh!.complete({'panel': {'name': 'old panel'}});
    await request;
    expect(state.bootstrap['panel']['name'], 'new panel');
  });
}
