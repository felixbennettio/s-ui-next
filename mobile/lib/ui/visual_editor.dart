import 'dart:convert';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:provider/provider.dart';

import '../core/app_locale_context.dart';
import '../state/app_state.dart';
import 'widgets.dart';

enum _EditorMode { visual, json }

class VisualEditorDialog extends StatefulWidget {
  const VisualEditorDialog({
    super.key,
    required this.title,
    required this.resource,
    required this.initialValue,
    required this.onSave,
    this.onSaveOnly,
    this.actionLabel = '',
    this.initialJson = false,
  });

  final String title;
  final String resource;
  final dynamic initialValue;
  final Future<void> Function(dynamic value) onSave;
  final Future<void> Function(dynamic value)? onSaveOnly;
  final String actionLabel;
  final bool initialJson;

  @override
  State<VisualEditorDialog> createState() => _VisualEditorDialogState();
}

class _VisualEditorDialogState extends State<VisualEditorDialog> {
  late dynamic value;
  late final TextEditingController jsonController;
  late final VisualEditorSchema schema;
  _EditorMode mode = _EditorMode.visual;
  bool saving = false;
  bool copyingSecret = false;
  String? error;

  @override
  void initState() {
    super.initState();
    schema = VisualEditorSchema.forResource(widget.resource);
    mode = widget.initialJson ? _EditorMode.json : _EditorMode.visual;
    value = _copy(widget.initialValue);
    jsonController = TextEditingController(text: prettyJson(value));
  }

  @override
  void dispose() {
    jsonController.dispose();
    super.dispose();
  }

  void changeMode(_EditorMode next) {
    if (next == mode) return;
    if (next == _EditorMode.visual) {
      try {
        value = jsonDecode(jsonController.text);
        error = null;
      } catch (exception) {
        setState(() => error = context.tr('editor.jsonError', args: {'error': exception}));
        return;
      }
    } else {
      jsonController.text = prettyJson(value);
      error = null;
    }
    setState(() => mode = next);
  }

  Future<void> save({bool apply = true}) async {
    if (saving) return;
    dynamic next = value;
    if (mode == _EditorMode.json) {
      try {
        next = jsonDecode(jsonController.text);
      } catch (exception) {
        setState(() => error = context.tr('editor.jsonError', args: {'error': exception}));
        return;
      }
    }
    setState(() {
      saving = true;
      error = null;
    });
    try {
      if (!apply && widget.onSaveOnly != null) {
        await widget.onSaveOnly!(next);
      } else {
        await widget.onSave(next);
      }
      if (mounted) Navigator.pop(context, true);
    } catch (exception) {
      if (mounted) setState(() => error = exception.toString());
    } finally {
      if (mounted) setState(() => saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final screen = MediaQuery.sizeOf(context);
    final compact = screen.width < 600;
    final editor = Scaffold(
      backgroundColor: Theme.of(context).colorScheme.surface,
        appBar: AppBar(
          title: Text(widget.title),
          leading: IconButton(icon: const Icon(Icons.close), onPressed: saving ? null : () => Navigator.pop(context)),
        ),
        bottomNavigationBar: SafeArea(
          minimum: const EdgeInsets.all(12),
          child: OverflowBar(
            alignment: MainAxisAlignment.end,
            spacing: 8,
            overflowSpacing: 8,
            children: [
            if (widget.onSaveOnly != null)
              TextButton.icon(
                onPressed: saving ? null : () => save(apply: false),
                icon: const Icon(Icons.save_outlined),
                label: Text(context.t('common.save')),
              ),
            TextButton.icon(
              onPressed: saving ? null : () => save(),
              icon: saving
                  ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.save_outlined),
              label: Text(
                widget.actionLabel.isNotEmpty
                    ? widget.actionLabel
                    : widget.onSaveOnly != null
                        ? context.t('common.saveAndApply')
                        : context.t('common.save'),
              ),
            ),
            ],
          ),
        ),
        body: SafeArea(
          child: Column(
            children: [
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 8, 12, 6),
                child: SegmentedButton<_EditorMode>(
                  segments: [
                    ButtonSegment(value: _EditorMode.visual, icon: const Icon(Icons.tune), label: Text(context.t('editor.visual'))),
                    const ButtonSegment(value: _EditorMode.json, icon: Icon(Icons.data_object), label: Text('JSON')),
                  ],
                  selected: {mode},
                  onSelectionChanged: saving ? null : (selection) => changeMode(selection.first),
                ),
              ),
              if (error != null)
                Padding(
                  padding: const EdgeInsets.fromLTRB(16, 4, 16, 8),
                  child: Text(error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
                ),
              Expanded(
                child: AbsorbPointer(
                  absorbing: saving,
                  child: mode == _EditorMode.json
                    ? Padding(
                        padding: const EdgeInsets.all(12),
                        child: TextField(
                          controller: jsonController,
                          expands: true,
                          maxLines: null,
                          minLines: null,
                          textAlignVertical: TextAlignVertical.top,
                          autocorrect: false,
                          enableSuggestions: false,
                          style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
                          decoration: const InputDecoration(hintText: '{}', alignLabelWithHint: true),
                        ),
                      )
                    : value is Map
                        ? _buildVisualBody()
                        : EmptyState(label: context.t('editor.needObject')),
                ),
              ),
            ],
          ),
        ),
      );
    if (compact) {
      return PopScope(canPop: !saving, child: Dialog.fullscreen(child: editor));
    }
    return PopScope(canPop: !saving, child: Dialog(
      insetPadding: const EdgeInsets.all(24),
      clipBehavior: Clip.antiAlias,
      child: SizedBox(
        width: math.min(960, screen.width - 48),
        height: math.min(820, screen.height - 48),
        child: editor,
      ),
    ));
  }

  Widget _buildVisualBody() {
    try {
      final root = _stringKeyMap(value as Map);
      value = root;
      return ListView(
        padding: const EdgeInsets.fromLTRB(12, 4, 12, 28),
        children: [
          Text(
            context.t('editor.visualHint'),
            style: Theme.of(context).textTheme.bodySmall?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant),
          ),
          const SizedBox(height: 10),
          ..._buildMap(root, ''),
          _addFieldButton(root, ''),
        ],
      );
    } catch (exception) {
      return EmptyState(label: exception.toString(), icon: Icons.error_outline);
    }
  }

  List<Widget> _buildMap(Map<String, dynamic> map, String path) {
    final entries = map.entries.where((entry) => !schema.isHiddenField(entry.key)).toList()
      ..sort((a, b) {
        final byOrder = schema.orderOf(path, a.key).compareTo(schema.orderOf(path, b.key));
        return byOrder == 0 ? a.key.compareTo(b.key) : byOrder;
      });
    return [for (final entry in entries) _buildField(map, entry.key, entry.value, path)];
  }

  Widget _buildField(Map<dynamic, dynamic> parent, String key, dynamic fieldValue, String parentPath) {
    final path = parentPath.isEmpty ? key : '$parentPath.$key';
    final label = context.fieldLabel(key);
    final options = schema.optionsFor(path, key, parent);

    if (schema.isStringBoolean(path, key, fieldValue)) {
      final checked = fieldValue.toString().toLowerCase() == 'true';
      return Card(
        margin: const EdgeInsets.only(bottom: 8),
        child: SwitchListTile.adaptive(
          title: Text(label),
          subtitle: Text(key),
          value: checked,
          onChanged: (next) => setState(() => parent[key] = next.toString()),
          secondary: _removeButton(parent, key),
        ),
      );
    }

    if (fieldValue is bool) {
      return Card(
        margin: const EdgeInsets.only(bottom: 8),
        child: SwitchListTile.adaptive(
          title: Text(label),
          subtitle: Text(key),
          value: fieldValue,
          onChanged: (next) => setState(() => parent[key] = next),
          secondary: _removeButton(parent, key),
        ),
      );
    }

    if (fieldValue is Map) {
      final child = Map<String, dynamic>.from(fieldValue);
      parent[key] = child;
      return Card(
        margin: const EdgeInsets.only(bottom: 8),
        child: ExpansionTile(
          initiallyExpanded: schema.expandByDefault(path),
          leading: const Icon(Icons.account_tree_outlined),
          title: Text(label),
          subtitle: Text('$key · ${context.t('editor.fieldsCount', args: {'count': child.length})}'),
          trailing: _removeButton(parent, key),
          childrenPadding: const EdgeInsets.fromLTRB(12, 0, 12, 12),
          children: [..._buildMap(child, path), _addFieldButton(child, path)],
        ),
      );
    }

    if (fieldValue is List) {
      return _buildList(parent, key, fieldValue, path, label);
    }

    if (schema.isWireGuardKeyField(path, key, parent, value)) {
      return _buildWireGuardKeyField(parent, key, fieldValue, path, label);
    }

    if (options != null && options.isNotEmpty) {
      final current = fieldValue?.toString() ?? '';
      final values = <String>{...options, if (current.isNotEmpty) current}.toList();
      return Card(
        margin: const EdgeInsets.only(bottom: 8),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(12, 10, 4, 10),
          child: Row(
            children: [
              Expanded(
                child: AnchoredSelect<String>(
                  key: ValueKey('$path:$current'),
                  value: current,
                  label: label,
                  options: [
                    for (final option in values)
                      SelectOption(
                        option,
                        context.t('option.$option') == 'option.$option' ? schema.optionLabel(option) : context.t('option.$option'),
                      ),
                  ],
                  onChanged: (next) {
                    setState(() {
                      if (parentPath.isEmpty && key == 'type' && value is Map) {
                        schema.applyRootType(value as Map<String, dynamic>, next);
                      } else {
                        parent[key] = schema.parseOption(fieldValue, next);
                      }
                    });
                  },
                ),
              ),
              _removeButton(parent, key),
            ],
          ),
        ),
      );
    }

    final isNumber = fieldValue is num || schema.isStringNumber(path, key, fieldValue);
    final multiline = schema.isMultiline(path, key);
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 10, 4, 10),
        child: Row(
          crossAxisAlignment: multiline ? CrossAxisAlignment.start : CrossAxisAlignment.center,
          children: [
            Expanded(
              child: TextFormField(
                key: ValueKey('$path:${fieldValue.runtimeType}'),
                initialValue: fieldValue?.toString() ?? '',
                minLines: multiline ? 4 : 1,
                maxLines: multiline ? 12 : 1,
                keyboardType: isNumber ? const TextInputType.numberWithOptions(decimal: true, signed: true) : TextInputType.text,
                inputFormatters: isNumber ? [FilteringTextInputFormatter.allow(RegExp(r'^-?\d*\.?\d*'))] : null,
                autocorrect: !schema.isSensitive(key),
                enableSuggestions: !schema.isSensitive(key),
                decoration: InputDecoration(labelText: label, helperText: key, alignLabelWithHint: multiline),
                onChanged: (next) {
                  parent[key] = schema.parseText(fieldValue, path, key, next);
                  if (parentPath.isEmpty && key == 'name' && value is Map<String, dynamic>) {
                    schema.syncClientName(value as Map<String, dynamic>, next);
                  }
                },
              ),
            ),
            _removeButton(parent, key),
          ],
        ),
      ),
    );
  }

  Widget _buildWireGuardKeyField(Map<dynamic, dynamic> parent, String key, dynamic fieldValue, String path, String label) {
    final current = fieldValue?.toString() ?? '';
    final redacted = schema.isRedactedSecret(current);
    final isPsk = key == 'pre_shared_key';
    final rootType = value is Map ? (value as Map)['type']?.toString() : '';
    final rootId = value is Map ? int.tryParse('${(value as Map)['id']}') ?? 0 : 0;
    final canCopyStored = rootType == 'wireguard' && rootId > 0 &&
        const {'private_key', 'client_private_key', 'pre_shared_key'}.contains(key) &&
        (redacted || boolValue(parent['${key}_set'])) &&
        (key == 'private_key' || (parent['public_key']?.toString().isNotEmpty ?? false));
    final canCopy = (current.isNotEmpty && !redacted) || canCopyStored;
    final canGeneratePair = rootType == 'warp'
        ? key == 'private_key'
        : key == 'private_key' || key == 'client_private_key' || key == 'public_key';
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: Padding(
        padding: const EdgeInsets.fromLTRB(12, 10, 4, 10),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            TextFormField(
                key: ValueKey('$path:$current'),
                initialValue: current,
                obscureText: key != 'public_key',
                autocorrect: false,
                enableSuggestions: false,
                decoration: InputDecoration(
                  labelText: label,
                  helperText: key,
                  suffixIcon: redacted ? const Icon(Icons.visibility_off_outlined) : null,
                ),
                onChanged: (next) {
                  parent[key] = next.trim();
                  if (isPsk) {
                    parent['pre_shared_key_set'] = next.trim().isNotEmpty;
                    parent.remove('pre_shared_key_clear');
                  }
                },
              ),
            Wrap(
              spacing: 2,
              children: [
                IconButton(
                  tooltip: context.t('resource.copy'),
                  onPressed: canCopy && !copyingSecret
                      ? () => _copyWireGuardKey(parent, key, rootId)
                      : null,
                  icon: const Icon(Icons.content_copy_outlined),
                ),
                if (canGeneratePair)
                  IconButton(
                    tooltip: context.t(current.isEmpty || redacted ? 'editor.generateKeyPair' : 'editor.regenerateKeyPair'),
                    onPressed: () => _generateWireGuardKeyPair(parent, key, path, current),
                    icon: const Icon(Icons.key_outlined),
                  ),
                if (isPsk) ...[
                  IconButton(
                    tooltip: context.t(current.isEmpty || redacted ? 'editor.generatePsk' : 'editor.regeneratePsk'),
                    onPressed: () => _generateWireGuardPsk(parent, current),
                    icon: const Icon(Icons.enhanced_encryption_outlined),
                  ),
                  IconButton(
                    tooltip: context.t('editor.clearPsk'),
                    onPressed: current.isEmpty && !boolValue(parent['pre_shared_key_set'])
                        ? null
                        : () {
                            setState(() {
                              parent[key] = '';
                              parent['pre_shared_key_set'] = false;
                              parent['pre_shared_key_clear'] = true;
                            });
                          },
                    icon: const Icon(Icons.clear_outlined),
                  ),
                ],
                _removeButton(parent, key),
              ],
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _copyWireGuardKey(Map<dynamic, dynamic> parent, String key, int endpointId) async {
    setState(() => copyingSecret = true);
    try {
      var secret = parent[key]?.toString() ?? '';
      if (secret.isEmpty || schema.isRedactedSecret(secret)) {
        final result = await context.read<AppState>().api!.post('wireguard/secret', data: {
          'id': endpointId,
          'field': key,
          if (key != 'private_key') 'publicKey': parent['public_key'],
        });
        if (!mounted) return;
        if (result is! String || result.isEmpty || schema.isRedactedSecret(result)) {
          throw FormatException(context.tr('common.copyFailed'));
        }
        secret = result;
      }
      // Deliberately do not retain the fetched secret in the editable resource.
      if (mounted) await copyText(context, secret);
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    } finally {
      if (mounted) setState(() => copyingSecret = false);
    }
  }

  Future<void> _generateWireGuardKeyPair(Map<dynamic, dynamic> parent, String key, String path, String current) async {
    final api = context.read<AppState>().api!;
    final regenerateTitle = context.t('editor.regenerateKeyPair');
    final regenerateMessage = context.t('editor.regenerateSecretMessage');
    final invalidMessage = context.t('editor.keypairInvalid');
    if (current.isNotEmpty || boolValue(parent['client_private_key_set']) || boolValue(parent['private_key_set'])) {
      final accepted = await confirm(context, title: regenerateTitle, message: regenerateMessage);
      if (!accepted || !mounted) return;
    }
    try {
      final result = await api.post('tools/keypair', data: {'type': 'wireguard', 'options': ''});
      if (!mounted) return;
      final values = result is List ? result.map((item) => item.toString()).toList() : <String>[];
      final privateKey = _lineValue(values, 'PrivateKey:');
      final publicKey = _lineValue(values, 'PublicKey:');
      if (privateKey.isEmpty || publicKey.isEmpty) throw FormatException(invalidMessage);
      setState(() {
        if (key == 'private_key' || path == 'ext.public_key') {
          final root = value is Map<String, dynamic> ? value as Map<String, dynamic> : parent;
          root['private_key'] = privateKey;
          root['private_key_set'] = true;
          final ext = Map<String, dynamic>.from(root['ext'] is Map ? root['ext'] as Map : const {});
          ext['public_key'] = publicKey;
          root['ext'] = ext;
        } else {
          parent['peer_key_mode'] = 'generated_client';
          parent['client_private_key'] = privateKey;
          parent['client_private_key_set'] = true;
          parent['public_key'] = publicKey;
        }
      });
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    }
  }

  Future<void> _generateWireGuardPsk(Map<dynamic, dynamic> parent, String current) async {
    final api = context.read<AppState>().api!;
    final regenerateTitle = context.t('editor.regeneratePsk');
    final regenerateMessage = context.t('editor.regenerateSecretMessage');
    final invalidMessage = context.t('editor.pskInvalid');
    if (current.isNotEmpty || boolValue(parent['pre_shared_key_set'])) {
      final accepted = await confirm(context, title: regenerateTitle, message: regenerateMessage);
      if (!accepted || !mounted) return;
    }
    try {
      final result = await api.post('tools/keypair', data: {'type': 'wireguard-psk', 'options': ''});
      if (!mounted) return;
      final values = result is List ? result.map((item) => item.toString()).toList() : <String>[];
      final psk = _lineValue(values, 'PresharedKey:');
      if (psk.isEmpty) throw FormatException(invalidMessage);
      setState(() {
        parent['pre_shared_key'] = psk;
        parent['pre_shared_key_set'] = true;
        parent.remove('pre_shared_key_clear');
      });
    } catch (exception) {
      if (mounted) showMessage(context, exception.toString(), error: true);
    }
  }

  Widget _buildList(Map<dynamic, dynamic> parent, String key, List<dynamic> list, String path, String label) {
    final objectList = list.any((item) => item is Map) || schema.isObjectList(path);
    if (!objectList) {
      final current = list.map((item) => item.toString()).join('\n');
      return Card(
        margin: const EdgeInsets.only(bottom: 8),
        child: Padding(
          padding: const EdgeInsets.fromLTRB(12, 10, 4, 10),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Expanded(
                child: TextFormField(
                  key: ValueKey('$path:${list.length}'),
                  initialValue: current,
                  minLines: 2,
                  maxLines: 8,
                  decoration: InputDecoration(labelText: label, helperText: '$key · ${context.t('editor.onePerLine')}', alignLabelWithHint: true),
                  onChanged: (next) {
                    final lines = next.split('\n').map((item) => item.trim()).where((item) => item.isNotEmpty);
                    parent[key] = [for (final line in lines) schema.parseListItem(list, path, line)];
                  },
                ),
              ),
              _removeButton(parent, key),
            ],
          ),
        ),
      );
    }

    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      child: ExpansionTile(
        initiallyExpanded: list.length <= 3,
        leading: const Icon(Icons.view_list_outlined),
        title: Text(label),
        subtitle: Text('$key · ${context.t('editor.itemsCount', args: {'count': list.length})}'),
        trailing: _removeButton(parent, key),
        childrenPadding: const EdgeInsets.fromLTRB(12, 0, 12, 12),
        children: [
          for (var index = 0; index < list.length; index++)
            Card.outlined(
              child: Padding(
                padding: const EdgeInsets.all(10),
                child: Column(
                  children: [
                    Row(
                      children: [
                        Expanded(child: Text('${context.singularFieldLabel(key)} ${index + 1}', style: const TextStyle(fontWeight: FontWeight.w700))),
                        IconButton(
                          tooltip: context.t('editor.deleteItem'),
                          onPressed: () => setState(() => list.removeAt(index)),
                          icon: const Icon(Icons.delete_outline),
                        ),
                      ],
                    ),
                    if (list[index] is Map) ..._buildMap(_stringKeyMap(list[index] as Map)..also((map) => list[index] = map), '$path[$index]') else TextFormField(
                      initialValue: list[index]?.toString() ?? '',
                      onChanged: (next) => list[index] = next,
                    ),
                  ],
                ),
              ),
            ),
          Align(
            alignment: Alignment.centerLeft,
            child: TextButton.icon(
              onPressed: () => setState(() => list.add(_copy(schema.listItemDefault(path, root: value)))),
              icon: const Icon(Icons.add),
              label: Text(context.t('editor.addItem', args: {'item': context.singularFieldLabel(key)})),
            ),
          ),
        ],
      ),
    );
  }

  Widget _removeButton(Map<dynamic, dynamic> parent, String key) => IconButton(
        tooltip: context.t('editor.deleteField'),
        onPressed: () => setState(() => parent.remove(key)),
        icon: const Icon(Icons.remove_circle_outline),
      );

  Widget _addFieldButton(Map<dynamic, dynamic> parent, String path) => Align(
        alignment: Alignment.centerLeft,
        child: TextButton.icon(
          onPressed: () => _addField(parent, path),
          icon: const Icon(Icons.add_circle_outline),
          label: Text(context.t('editor.addField')),
        ),
      );

  Future<void> _addField(Map<dynamic, dynamic> parent, String path) async {
    final keyController = TextEditingController();
    var kind = 'text';
    String? suggestedKey;
    final missing = schema.suggestedKeys(path, root: value).where((key) => !parent.containsKey(key)).toList();
    final result = await showDialog<Map<String, String>>(
      context: context,
      builder: (dialogContext) => StatefulBuilder(
        builder: (context, setDialogState) => AlertDialog(
          title: Text(context.t('editor.addConfigField')),
          content: SizedBox(
            width: 440,
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                if (missing.isNotEmpty)
                  AnchoredSelect<String>(
                    value: suggestedKey,
                    label: context.t('editor.commonField'),
                    options: [for (final item in missing) SelectOption(item, '${context.fieldLabel(item)} · $item')],
                    onChanged: (next) {
                      setDialogState(() => suggestedKey = next);
                      keyController.text = next;
                    },
                  ),
                const SizedBox(height: 10),
                TextField(controller: keyController, decoration: InputDecoration(labelText: context.t('editor.fieldName'))),
                const SizedBox(height: 10),
                AnchoredSelect<String>(
                  value: kind,
                  label: context.t('editor.valueType'),
                  options: [
                    SelectOption('text', context.t('editor.text')),
                    SelectOption('number', context.t('editor.number')),
                    SelectOption('bool', context.t('editor.bool')),
                    SelectOption('object', context.t('editor.object')),
                    SelectOption('list', context.t('editor.list')),
                  ],
                  onChanged: (next) => setDialogState(() => kind = next),
                ),
              ],
            ),
          ),
          actions: [
            TextButton(onPressed: () => Navigator.pop(dialogContext), child: Text(context.t('common.cancel'))),
            FilledButton(
              onPressed: () => Navigator.pop(dialogContext, {'key': keyController.text.trim(), 'kind': kind}),
              child: Text(context.t('common.confirm')),
            ),
          ],
        ),
      ),
    );
    keyController.dispose();
    if (result == null || result['key']?.isEmpty != false) return;
    final key = result['key']!;
    setState(() {
      parent[key] = switch (result['kind']) {
        'number' => 0,
        'bool' => false,
        'object' => <String, dynamic>{},
        'list' => <dynamic>[],
        _ => '',
      };
    });
  }
}

extension _Also<T> on T {
  T also(void Function(T value) action) {
    action(this);
    return this;
  }
}

dynamic _copy(dynamic value) => jsonDecode(jsonEncode(value));

Map<String, dynamic> _stringKeyMap(Map<dynamic, dynamic> value) {
  final result = <String, dynamic>{};
  for (final entry in value.entries) {
    result[entry.key.toString()] = entry.value;
  }
  return result;
}

bool boolValue(dynamic value) => value == true || value?.toString().toLowerCase() == 'true';

String _lineValue(List<String> lines, String prefix) {
  for (final line in lines) {
    if (line.startsWith(prefix)) return line.substring(prefix.length).trim();
  }
  return '';
}

class VisualEditorSchema {
  VisualEditorSchema._(this.resource);

  factory VisualEditorSchema.forResource(String resource) => VisualEditorSchema._(resource);

  final String resource;

  static const _typeOptions = <String, List<String>>{
    'inbounds': ['direct', 'mixed', 'socks', 'http', 'shadowsocks', 'vmess', 'trojan', 'naive', 'hysteria', 'shadowtls', 'tuic', 'hysteria2', 'vless', 'anytls', 'tun', 'redirect', 'tproxy'],
    'outbounds': ['direct', 'socks', 'http', 'shadowsocks', 'vmess', 'trojan', 'naive', 'hysteria', 'vless', 'shadowtls', 'tuic', 'hysteria2', 'anytls', 'tor', 'ssh', 'selector', 'urltest'],
    'endpoints': ['wireguard', 'warp', 'tailscale'],
    'services': ['derp', 'resolved', 'ssm-api', 'ocm', 'ccm'],
  };

  static const _order = [
    'id', 'enable', 'enabled', 'type', 'tag', 'name', 'group', 'desc', 'listen', 'listen_port', 'server', 'server_port',
    'username', 'user', 'password', 'uuid', 'method', 'network', 'version', 'tls_id', 'transport', 'tls', 'multiplex',
    'config', 'inbounds', 'outbounds', 'links', 'volume', 'expiry', 'autoReset', 'resetDays', 'delayStart',
  ];

  String optionLabel(String option) => const {'ws': 'WebSocket', 'grpc': 'gRPC', 'httpupgrade': 'HTTP Upgrade', 'urltest': 'URL Test', 'ssm-api': 'SSM API'}[option] ?? option;
  int orderOf(String path, String key) {
    final index = _order.indexOf(key);
    return index < 0 ? 1000 : index;
  }

  bool expandByDefault(String path) => const ['config', 'server', 'client', 'tls', 'transport'].contains(path) || path.split('.').length <= 1;
  bool isSensitive(String key) =>
      key.contains('password') ||
      key.contains('secret') ||
      key.contains('private_key') ||
      key.contains('token') ||
      key.contains('license') ||
      key == 'auth_key' ||
      key == 'key';
  bool isMultiline(String path, String key) => key == 'certificate' || key == 'key' || key == 'private_key' || key.endsWith('Ext') || key == 'content';
  bool isStringBoolean(String path, String key, dynamic value) => resource == 'settings' && const {'subEncode', 'subShowInfo'}.contains(key);
  bool isStringNumber(String path, String key, dynamic value) => resource == 'settings' && const {'webPort', 'subPort', 'sessionMaxAge', 'trafficAge', 'subUpdates'}.contains(key);
  bool isHiddenField(String key) => const {
    'private_key_set',
    'client_private_key_set',
    'pre_shared_key_set',
    'pre_shared_key_clear',
    'access_token_set',
    'license_key_set',
  }.contains(key);
  bool isRedactedSecret(String value) => value == '[redacted]' || value.contains('•');
  bool isWireGuardKeyField(String path, String key, Map<dynamic, dynamic> parent, dynamic root) {
    if (resource != 'endpoints' || root is! Map) return false;
    final type = root['type']?.toString();
    if (type == 'wireguard') return const {'private_key', 'public_key', 'client_private_key', 'pre_shared_key'}.contains(key);
    if (type == 'warp') return const {'private_key', 'public_key', 'pre_shared_key'}.contains(key);
    return false;
  }

  List<String>? optionsFor(String path, String key, Map<dynamic, dynamic> parent) {
    if (path == 'type' && _typeOptions.containsKey(resource)) return _typeOptions[resource];
    if (key == 'type' && path.endsWith('transport.type')) return const ['http', 'ws', 'quic', 'grpc', 'httpupgrade'];
    if (key == 'type' && path.contains('dns.servers')) return const ['local', 'hosts', 'tcp', 'udp', 'tls', 'quic', 'https', 'h3', 'dhcp', 'fakeip', 'tailscale', 'resolved'];
    if (key == 'type' && path.contains('rule_set')) return const ['local', 'remote'];
    if (key == 'type' && path.contains('links')) return const ['local', 'external', 'sub'];
    if (key == 'type' && path.contains('rules')) return const ['simple', 'logical'];
    if (key == 'action') return const ['route', 'route-options', 'reject', 'hijack-dns', 'sniff', 'resolve', 'bypass', 'predefined'];
    if (key == 'mode') return const ['and', 'or', 'rule', 'global', 'direct'];
    if (key == 'peer_mode') return const ['roaming_client', 'static_peer', 'site_to_site'];
    if (key == 'peer_role') return const ['client', 'fixed_node', 'site_gateway'];
    if (key == 'peer_key_mode') return const ['generated_client', 'existing_peer'];
    if (key == 'remote_endpoint_mode') return const ['dynamic', 'static'];
    if (key == 'runtime_route_preset') return const ['peer_addresses', 'remote_networks', 'custom', 'full_tunnel'];
    if (key == 'client_route_preset') return const ['virtual_network', 'single_peer', 'custom', 'full_tunnel'];
    if (key == 'network') return const ['tcp', 'udp'];
    if (key == 'strategy') return const ['', 'prefer_ipv4', 'prefer_ipv6', 'ipv4_only', 'ipv6_only'];
    if (key == 'level') return const ['trace', 'debug', 'info', 'warn', 'error', 'fatal', 'panic'];
    if (key == 'packet_encoding') return const ['', 'packetaddr', 'xudp'];
    if (key == 'congestion_control' || key == 'quic_congestion_control') return const ['', 'cubic', 'new_reno', 'bbr', 'bbr2', 'reno'];
    if (key == 'method' && (parent['type'] == 'shadowsocks' || path.contains('shadowsocks'))) {
      return const ['none', 'aes-128-gcm', 'aes-256-gcm', 'chacha20-ietf-poly1305', '2022-blake3-aes-128-gcm', '2022-blake3-aes-256-gcm', '2022-blake3-chacha20-poly1305'];
    }
    if (key == 'flow') return const ['', 'xtls-rprx-vision'];
    if (key == 'fingerprint') return const ['chrome', 'firefox', 'edge', 'safari', 'ios', 'android', 'random', 'randomized'];
    if (key == 'min_version' || key == 'max_version') return const ['1.0', '1.1', '1.2', '1.3'];
    if (key == 'store') return const ['mozilla', 'chrome'];
    if (resource == 'settings' && key == 'timeLocation') return const ['Asia/Shanghai', 'Asia/Tehran', 'UTC', 'Local'];
    return null;
  }

  dynamic parseOption(dynamic oldValue, String next) {
    if (oldValue is int) return int.tryParse(next) ?? oldValue;
    if (oldValue is double) return double.tryParse(next) ?? oldValue;
    return next;
  }

  dynamic parseText(dynamic oldValue, String path, String key, String next) {
    if (isStringNumber(path, key, oldValue)) return next;
    if (oldValue is int) return int.tryParse(next) ?? 0;
    if (oldValue is double) return double.tryParse(next) ?? 0;
    return next;
  }

  dynamic parseListItem(List<dynamic> oldList, String path, String next) {
    if (oldList.isNotEmpty && oldList.first is int) return int.tryParse(next) ?? 0;
    if (oldList.isNotEmpty && oldList.first is double) return double.tryParse(next) ?? 0;
    final key = path.split('.').last;
    if (const {'inbounds', 'source_port', 'port', 'user_id', 'reserved', 'exclude_uid', 'include_uid', 'include_android_user'}.contains(key)) {
      return int.tryParse(next) ?? 0;
    }
    return next;
  }

  bool isObjectList(String path) =>
      path.endsWith('.peers') ||
      path.endsWith('.users') ||
      path.endsWith('.links') ||
      path.endsWith('.rules') ||
      path.endsWith('.rule_set') ||
      path.endsWith('.servers') ||
      path.endsWith('.verify_client_url') ||
      path.endsWith('.mesh_with') ||
      path == 'peers' ||
      path == 'users' ||
      path == 'links' ||
      path == 'rules' ||
      path == 'rule_set' ||
      path == 'servers' ||
      path == 'verify_client_url' ||
      path == 'mesh_with';

  dynamic listItemDefault(String path, {dynamic root}) {
    if (path.endsWith('peers') || path == 'peers') {
      if (resource == 'endpoints') {
        if (root is Map && root['type'] == 'warp') {
          return {
            'address': '',
            'port': 0,
            'public_key': '',
            'pre_shared_key': '',
            'reserved': <int>[],
            'allowed_ips': <String>[],
          };
        }
        return {
          'name': '', 'peer_role': 'fixed_node', 'peer_mode': 'static_peer', 'peer_key_mode': 'existing_peer', 'remote_endpoint_mode': 'dynamic',
          'public_key': '', 'client_private_key': '', 'pre_shared_key': '',
          'assigned_ipv4': '', 'assigned_ipv6': '', 'runtime_route_preset': 'custom', 'runtime_allowed_ips': <String>[], 'server_allowed_ips': <String>[], 'allowed_ips': <String>[],
          'remote_site_cidrs': <String>[], 'local_site_cidrs': <String>[], 'route_inbounds': <String>[],
          'client_route_preset': 'virtual_network', 'client_allowed_ips': <String>[], 'client_dns': <String>[],
          'client_mtu': 1420, 'client_keepalive': 25, 'include_ipv4': true, 'include_ipv6': true,
        };
      }
      return {'server': '', 'server_port': 443, 'public_key': '', 'allowed_ips': <String>[]};
    }
    if (path.endsWith('links') || path == 'links') return {'type': 'external', 'remark': '', 'uri': ''};
    if (path.endsWith('users') || path == 'users') return {'name': '', 'token': ''};
    if (path.endsWith('verify_client_url') || path == 'verify_client_url') return {'url': ''};
    if (path.endsWith('mesh_with') || path == 'mesh_with') return {'server': '', 'server_port': 443, 'tls': <String, dynamic>{}};
    if (path.endsWith('rule_set') || path == 'rule_set') return {'type': 'remote', 'tag': '', 'format': 'binary', 'url': ''};
    if (path.endsWith('rules') || path == 'rules') return {'action': 'route', 'outbound': '', 'invert': false};
    if (path.endsWith('servers') || path == 'servers') return {'type': 'local', 'tag': ''};
    return <String, dynamic>{};
  }

  List<String> suggestedKeys(String path, {dynamic root}) {
    if (path.endsWith('tls') || path == 'server' || path == 'client') return ['enabled', 'server_name', 'insecure', 'disable_sni', 'alpn', 'min_version', 'max_version', 'certificate', 'certificate_path', 'key', 'key_path', 'acme', 'reality', 'ech', 'utls'];
    if (path.endsWith('transport')) return ['type', 'host', 'path', 'method', 'headers', 'service_name', 'idle_timeout', 'ping_timeout'];
    if (path.endsWith('multiplex')) return ['enabled', 'protocol', 'padding', 'max_connections', 'min_streams', 'max_streams', 'brutal'];
    if (path.contains('route')) return ['rules', 'rule_set', 'final', 'auto_detect_interface', 'default_interface', 'default_mark', 'default_domain_resolver'];
    if (path.contains('dns')) return ['servers', 'rules', 'final', 'strategy', 'disable_cache', 'independent_cache', 'cache_capacity', 'reverse_mapping'];
    if (path.endsWith('peers') || path.contains('peers[')) {
      if (root is Map && root['type'] == 'warp') return ['address', 'port', 'public_key', 'pre_shared_key', 'reserved', 'allowed_ips'];
      return ['name', 'peer_role', 'peer_key_mode', 'remote_endpoint_mode', 'public_key', 'client_private_key', 'pre_shared_key', 'assigned_ipv4', 'assigned_ipv6', 'runtime_route_preset', 'runtime_allowed_ips', 'remote_site_cidrs', 'local_site_cidrs', 'route_inbounds', 'client_allowed_ips', 'client_dns'];
    }
    if (resource == 'endpoints' && root is Map) {
      if (root['type'] == 'warp') return ['warp_terms_accepted', 'address', 'private_key', 'listen_port', 'mtu', 'udp_timeout', 'workers', 'system', 'name', 'peers', 'ext'];
      if (root['type'] == 'tailscale') return ['domain_resolver', 'state_directory', 'auth_key', 'control_url', 'ephemeral', 'hostname', 'accept_routes', 'exit_node', 'advertise_routes', 'relay_server_port', 'relay_server_static_endpoints', 'system_interface', 'udp_timeout'];
    }
    if (resource == 'services' && root is Map) {
      switch (root['type']) {
        case 'derp':
          return ['listen', 'listen_port', 'config_path', 'tls_id', 'verify_client_endpoint', 'verify_client_url', 'home', 'mesh_with', 'mesh_psk', 'mesh_psk_file', 'stun'];
        case 'ssm-api':
          return ['listen', 'listen_port', 'tls_id', 'servers'];
        case 'ocm':
        case 'ccm':
          return ['listen', 'listen_port', 'tls_id', 'credential_path', 'usages_path', 'users', 'headers', 'detour'];
        case 'resolved':
          return ['listen', 'listen_port'];
      }
    }
    return ['type', 'tag', 'name', 'enabled', 'server', 'server_port', 'listen', 'listen_port', 'network', 'tls', 'transport', 'headers'];
  }

  dynamic defaultValue() {
    if (resource == 'clients') {
      final name = _randomText(8);
      return {
        'id': 0, 'enable': true, 'name': name, 'config': _clientConfig(name), 'inbounds': <int>[], 'links': <dynamic>[],
        'volume': 0, 'expiry': 0, 'up': 0, 'down': 0, 'desc': '', 'group': '', 'delayStart': false,
        'autoReset': false, 'resetDays': 0, 'nextReset': 0, 'totalUp': 0, 'totalDown': 0,
      };
    }
    if (resource == 'tls') return {'id': 0, 'name': '', 'server': {'enabled': true, 'alpn': ['h3', 'h2', 'http/1.1']}, 'client': {'enabled': true, 'utls': {'enabled': true, 'fingerprint': 'chrome'}}};
    final firstType = const {'inbounds': 'vless', 'outbounds': 'direct', 'endpoints': 'wireguard', 'services': 'resolved'}[resource] ?? _typeOptions[resource]?.first;
    return firstType == null ? <String, dynamic>{} : _rootTemplate(firstType);
  }

  void applyRootType(Map<String, dynamic> current, String type) {
    final next = _rootTemplate(type);
    final preserve = switch (resource) {
      'inbounds' => ['id', 'tag', 'listen', 'listen_port', 'tls_id', 'addrs', 'out_json'],
      'outbounds' => ['id', 'tag'],
      'endpoints' => ['id', 'tag'],
      'services' => ['id', 'tag', 'listen', 'listen_port', 'tls_id'],
      _ => <String>[],
    };
    for (final key in preserve) {
      if (current.containsKey(key)) next[key] = current[key];
    }
    current
      ..clear()
      ..addAll(next);
  }

  void syncClientName(Map<String, dynamic> current, String name) {
    if (resource != 'clients' || current['config'] is! Map) return;
    for (final raw in (current['config'] as Map).values) {
      if (raw is! Map) continue;
      if (raw.containsKey('name')) raw['name'] = name;
      if (raw.containsKey('username')) raw['username'] = name;
    }
  }

  Map<String, dynamic> _rootTemplate(String type) {
    if (resource == 'inbounds') {
      final base = <String, dynamic>{'id': 0, 'type': type, 'tag': '', 'listen': '::', 'listen_port': 443, 'tls_id': 0};
      final detail = <String, Map<String, dynamic>>{
        'direct': {'network': 'tcp'}, 'mixed': {}, 'socks': {}, 'http': {},
        'shadowsocks': {'method': 'none', 'password': '', 'network': 'tcp'},
        'vmess': {'transport': <String, dynamic>{}}, 'trojan': {'transport': <String, dynamic>{}},
        'naive': {'quic_congestion_control': ''}, 'hysteria': {'up_mbps': 100, 'down_mbps': 100},
        'shadowtls': {'version': 3, 'password': '', 'handshake': {'server': '', 'server_port': 443}},
        'tuic': {'congestion_control': 'cubic', 'auth_timeout': '3s', 'heartbeat': '10s'},
        'hysteria2': {'up_mbps': 100, 'down_mbps': 100},
        'vless': {'transport': <String, dynamic>{}},
        'anytls': {'padding_scheme': ['stop=8', '0=30-30', '1=100-400']},
        'tun': {'address': ['172.19.0.1/30', 'fdfe:dcba:9876::1/126'], 'mtu': 9000, 'stack': 'system', 'udp_timeout': '5m', 'auto_route': false, 'strict_route': false},
        'redirect': {}, 'tproxy': {'network': 'tcp'},
      };
      return {...base, ...?detail[type]};
    }
    if (resource == 'outbounds') {
      final base = <String, dynamic>{'id': 0, 'type': type, 'tag': ''};
      final server = {'server': '', 'server_port': 443};
      final detail = <String, Map<String, dynamic>>{
        'direct': {}, 'socks': {...server, 'version': '5', 'username': '', 'password': ''},
        'http': {...server, 'username': '', 'password': '', 'tls': <String, dynamic>{}},
        'shadowsocks': {...server, 'method': 'none', 'password': '', 'multiplex': <String, dynamic>{}},
        'vmess': {...server, 'uuid': '', 'security': 'auto', 'alter_id': 0, 'global_padding': false, 'tls': <String, dynamic>{}, 'multiplex': <String, dynamic>{}, 'transport': <String, dynamic>{}},
        'trojan': {...server, 'password': '', 'tls': <String, dynamic>{}, 'multiplex': <String, dynamic>{}, 'transport': <String, dynamic>{}},
        'naive': {...server, 'username': '', 'password': '', 'tls': {'enabled': true}},
        'hysteria': {...server, 'up_mbps': 100, 'down_mbps': 100, 'auth_str': '', 'tls': {'enabled': true}},
        'vless': {...server, 'uuid': '', 'flow': 'xtls-rprx-vision', 'tls': <String, dynamic>{}, 'multiplex': <String, dynamic>{}, 'transport': <String, dynamic>{}},
        'shadowtls': {...server, 'version': 3, 'password': '', 'tls': {'enabled': true}},
        'tuic': {...server, 'uuid': '', 'password': '', 'congestion_control': 'cubic', 'tls': {'enabled': true}},
        'hysteria2': {...server, 'password': '', 'hop_interval': '30s', 'tls': {'enabled': true}},
        'anytls': {...server, 'password': '', 'idle_session_check_interval': '30s', 'idle_session_timeout': '30s', 'min_idle_session': 0, 'tls': {'enabled': true}},
        'tor': {'executable_path': './tor', 'data_directory': r'$HOME/.cache/tor', 'torrc': {'ClientOnly': '1'}},
        'ssh': {...server, 'user': '', 'password': ''},
        'selector': {'outbounds': <String>[], 'default': '', 'interrupt_exist_connections': false},
        'urltest': {'outbounds': <String>[], 'url': 'https://www.gstatic.com/generate_204', 'interval': '3m', 'tolerance': 50},
      };
      return {...base, ...?detail[type]};
    }
    if (resource == 'endpoints') {
      final base = <String, dynamic>{'id': 0, 'type': type, 'tag': ''};
      final detail = <String, Map<String, dynamic>>{
        'wireguard': {
          'wireguard_schema': 4,
          'address': ['10.66.66.1/32', 'fd66:66:66::1/128'],
          'tunnel_ipv4_cidr': '10.66.66.0/24',
          'tunnel_ipv6_cidr': 'fd66:66:66::/64',
          'private_key': '',
          'listen_port': 0,
          'advertised_endpoint_host': '',
          'advertised_endpoint_port': 0,
          'client_export_enabled': true,
          'peer_to_peer_enabled': false,
          'hub_peer_forwarding_enabled': false,
          'default_client_allowed_ips': ['10.66.66.0/24', 'fd66:66:66::/64'],
          'default_client_dns': <String>[],
          'default_client_mtu': 1420,
          'default_client_keepalive': 25,
          'system': false,
          'peers': <dynamic>[],
        },
        'warp': {'warp_terms_accepted': false, 'address': <String>[], 'private_key': '', 'listen_port': 0, 'mtu': 1280, 'system': false, 'peers': <dynamic>[], 'ext': <String, dynamic>{}},
        'tailscale': {'domain_resolver': 'local', 'accept_routes': false},
      };
      return {...base, ...?detail[type]};
    }
    if (resource == 'services') {
      final detail = <String, Map<String, dynamic>>{
        'derp': {'listen': '::', 'listen_port': 8443, 'config_path': '', 'tls_id': 0},
        'resolved': {'listen': '::', 'listen_port': 53},
        'ssm-api': {'listen': '::', 'listen_port': 8080, 'tls_id': 0, 'servers': <String, dynamic>{}},
        'ocm': {'listen': '::', 'listen_port': 8080, 'tls_id': 0, 'credential_path': '', 'usages_path': '', 'users': <dynamic>[], 'headers': <String, dynamic>{}},
        'ccm': {'listen': '::', 'listen_port': 8080, 'tls_id': 0, 'credential_path': '', 'usages_path': '', 'users': <dynamic>[], 'headers': <String, dynamic>{}},
      };
      return {'id': 0, 'type': type, 'tag': '', ...?detail[type]};
    }
    return {'type': type};
  }

  Map<String, dynamic> _clientConfig(String name) {
    final password = _randomText(12);
    final uuid = _uuid();
    return {
      'mixed': {'username': name, 'password': password}, 'socks': {'username': name, 'password': password}, 'http': {'username': name, 'password': password},
      'shadowsocks': {'name': name, 'password': _randomText(44)}, 'shadowsocks16': {'name': name, 'password': _randomText(24)}, 'shadowtls': {'name': name, 'password': _randomText(44)},
      'vmess': {'name': name, 'uuid': uuid, 'alterId': 0}, 'vless': {'name': name, 'uuid': uuid, 'flow': 'xtls-rprx-vision'},
      'anytls': {'name': name, 'password': password}, 'trojan': {'name': name, 'password': password}, 'naive': {'username': name, 'password': password},
      'hysteria': {'name': name, 'auth_str': password}, 'tuic': {'name': name, 'uuid': _uuid(), 'password': password}, 'hysteria2': {'name': name, 'password': password},
    };
  }

  String _randomText(int length) {
    const alphabet = 'abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789';
    final random = math.Random.secure();
    return List.generate(length, (_) => alphabet[random.nextInt(alphabet.length)]).join();
  }

  String _uuid() {
    final random = math.Random.secure();
    final bytes = List<int>.generate(16, (_) => random.nextInt(256));
    bytes[6] = (bytes[6] & 0x0f) | 0x40;
    bytes[8] = (bytes[8] & 0x3f) | 0x80;
    final hex = bytes.map((value) => value.toRadixString(16).padLeft(2, '0')).join();
    return '${hex.substring(0, 8)}-${hex.substring(8, 12)}-${hex.substring(12, 16)}-${hex.substring(16, 20)}-${hex.substring(20)}';
  }
}
