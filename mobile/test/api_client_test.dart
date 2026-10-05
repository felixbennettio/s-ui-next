import 'dart:convert';
import 'dart:io';

import 'package:flutter_test/flutter_test.dart';
import 'package:sui_mobile/core/api_client.dart';
import 'package:sui_mobile/core/connection_profile.dart';

void main() {
  test('HTTP login and authenticated requests preserve the panel base path', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(() => server.close(force: true));
    final paths = <String>[];
    server.listen((request) async {
      paths.add(request.uri.path);
      request.response.headers.contentType = ContentType.json;
      if (request.uri.path == '/app/apiv3/auth/login') {
        expect(request.method, 'POST');
        final body = jsonDecode(await utf8.decoder.bind(request).join()) as Map;
        expect(body['username'], 'test-admin');
        request.response.write(jsonEncode({'success': true, 'data': {'token': 'test-only-token'}}));
      } else {
        expect(request.headers.value('Authorization'), 'Bearer test-only-token');
        request.response.write(jsonEncode({'success': true, 'data': {'apiVersion': '3'}}));
      }
      await request.response.close();
    });
    final profile = ConnectionProfile(name: 'LAN panel', baseUrl: 'http://127.0.0.1:${server.port}/app');
    final login = await ApiClient.login(profile: profile, username: 'test-admin', password: 'test-only-password');
    final client = ApiClient(profile.copyWith(token: login['token'] as String));
    expect(await client.get('meta'), {'apiVersion': '3'});
    expect(paths, ['/app/apiv3/auth/login', '/app/apiv3/meta']);
  });

  test('an HTTP authentication failure remains distinct from a connection failure', () async {
    final server = await HttpServer.bind(InternetAddress.loopbackIPv4, 0);
    addTearDown(() => server.close(force: true));
    server.listen((request) async {
      request.response.statusCode = HttpStatus.unauthorized;
      request.response.headers.contentType = ContentType.json;
      request.response.write(jsonEncode({'success': false, 'error': 'invalid or expired API token'}));
      await request.response.close();
    });
    final client = ApiClient(ConnectionProfile(name: 'LAN panel', baseUrl: 'http://127.0.0.1:${server.port}/app/'));
    await expectLater(client.get('meta'), throwsA(isA<ApiException>()
        .having((error) => error.statusCode, 'status code', 401)
        .having((error) => error.message, 'message', 'invalid or expired API token')));
  });
}
