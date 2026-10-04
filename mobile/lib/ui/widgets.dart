import 'dart:convert';
import 'dart:math' as math;
import 'dart:ui' as ui;

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:intl/intl.dart';

import '../core/app_locale_context.dart';
import '../core/save_result.dart';

void showSaveResult(BuildContext context, SaveResult result) {
  showMessage(context, context.tr(result.messageKey));
}

Future<void> copyText(BuildContext context, String value) async {
  try {
    await Clipboard.setData(ClipboardData(text: value));
    if (context.mounted) showMessage(context, context.tr('resource.copied'));
  } catch (_) {
    if (context.mounted) showMessage(context, context.tr('common.copyFailed'), error: true);
  }

const _selectMenuRadius = BorderRadius.all(Radius.circular(16));
const _selectMenuRowHeight = kMinInteractiveDimension;
const _selectMenuMaxRows = 5;

class SelectOption<T> {
  const SelectOption(this.value, this.label);

  final T value;
  final String label;
}

class AnchoredSelect<T> extends StatefulWidget {
  const AnchoredSelect({
    super.key,
    required this.value,
    required this.options,
    required this.onChanged,
    this.label,
    this.helperText,
    this.prefixIcon,
    this.compact = false,
  });

  final T? value;
  final String? label;
  final String? helperText;
  final Widget? prefixIcon;
  final List<SelectOption<T>> options;
  final ValueChanged<T> onChanged;
  final bool compact;

  @override
  State<AnchoredSelect<T>> createState() => _AnchoredSelectState<T>();
}

class _AnchoredSelectState<T> extends State<AnchoredSelect<T>> {
  final LayerLink _layerLink = LayerLink();
  final GlobalKey _targetKey = GlobalKey();
  final ScrollController _scrollController = ScrollController();
  OverlayEntry? _overlayEntry;
  double _menuWidth = 0;
  double _menuHeight = 0;

  bool get _isOpen => _overlayEntry != null;

  @override
  void dispose() {
    _overlayEntry?.remove();
    _overlayEntry = null;
    _scrollController.dispose();
    super.dispose();
  }

  @override
  void didUpdateWidget(covariant AnchoredSelect<T> oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (_isOpen && oldWidget.options != widget.options) {
      _close();
    }
  }

  void _toggle() => _isOpen ? _close() : _open();

  void _open() {
    final target = _targetKey.currentContext?.findRenderObject() as RenderBox?;
    if (target == null || !target.hasSize || widget.options.isEmpty) return;
    final overlay = Overlay.of(context, rootOverlay: true).context.findRenderObject() as RenderBox?;
    if (overlay == null || !overlay.hasSize) return;
    final media = MediaQuery.of(context);
    final targetBottom = target.localToGlobal(Offset(0, target.size.height)).dy;
    final availableBelow = media.size.height - media.padding.bottom - targetBottom - 8;
    _menuWidth = target.size.width;
    _menuHeight = math.min(
      math.min(widget.options.length, _selectMenuMaxRows) * _selectMenuRowHeight,
      math.max(_selectMenuRowHeight, availableBelow),
    );
    _overlayEntry = OverlayEntry(builder: _buildOverlay);
    Overlay.of(context, rootOverlay: true).insert(_overlayEntry!);
    setState(() {});
  }

  void _close() {
    _overlayEntry?.remove();
    _overlayEntry = null;
    if (mounted) setState(() {});
  }

  Widget _buildOverlay(BuildContext overlayContext) {
    final rtl = Directionality.of(context) == ui.TextDirection.rtl;
    final colors = Theme.of(context).colorScheme;
    final followerAnchor = rtl ? Alignment.topRight : Alignment.topLeft;
    final targetAnchor = rtl ? Alignment.bottomRight : Alignment.bottomLeft;
    return Stack(
      children: [
        Positioned.fill(
          child: GestureDetector(
            behavior: HitTestBehavior.translucent,
            onTap: _close,
          ),
        ),
        Positioned.fill(
          child: CompositedTransformFollower(
            link: _layerLink,
            showWhenUnlinked: false,
            targetAnchor: targetAnchor,
            followerAnchor: followerAnchor,
            offset: const Offset(0, -1),
            child: Align(
              alignment: followerAnchor,
              widthFactor: 1,
              heightFactor: 1,
              child: SizedBox(
                key: const ValueKey('anchored-select-menu'),
                width: _menuWidth,
                height: _menuHeight,
                child: Material(
                  color: colors.surfaceContainerHigh,
                  elevation: 8,
                  shadowColor: colors.shadow,
                  shape: RoundedRectangleBorder(
                    borderRadius: _selectMenuRadius,
                    side: BorderSide(color: colors.outlineVariant),
                  ),
                  clipBehavior: Clip.antiAlias,
                  child: MediaQuery.removePadding(
                    context: overlayContext,
                    removeTop: true,
                    removeBottom: true,
                    child: Scrollbar(
                      controller: _scrollController,
                      thumbVisibility: widget.options.length > _selectMenuMaxRows,
                      trackVisibility: widget.options.length > _selectMenuMaxRows,
                      interactive: true,
                      thickness: 4,
                      radius: const Radius.circular(8),
                      child: ListView.builder(
                        controller: _scrollController,
                        padding: EdgeInsets.zero,
                        primary: false,
                        itemExtent: _selectMenuRowHeight,
                        itemCount: widget.options.length,
                        itemBuilder: (context, index) {
                          final option = widget.options[index];
                          final selected = option.value == widget.value;
                          return DecoratedBox(
                            decoration: BoxDecoration(
                              color: selected ? colors.secondaryContainer.withValues(alpha: .72) : null,
                              border: index == widget.options.length - 1 ? null : Border(bottom: BorderSide(color: colors.outlineVariant)),
                            ),
                            child: InkWell(
                              onTap: () {
                                _close();
                                widget.onChanged(option.value);
                              },
                              child: Padding(
                                padding: const EdgeInsetsDirectional.only(start: 12, end: 12),
                                child: Row(
                                  children: [
                                    Expanded(child: Text(option.label, maxLines: 1, overflow: TextOverflow.ellipsis)),
                                    if (selected) ...[
                                      const SizedBox(width: 8),
                                      const Icon(Icons.check, size: 20),
                                    ],
                                  ],
                                ),
                              ),
                            ),
                          );
                        },
                      ),
                    ),
                  ),
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }

  @override
  Widget build(BuildContext context) {
    SelectOption<T>? selected;
    for (final option in widget.options) {
      if (option.value == widget.value) {
        selected = option;
        break;
      }
    }

    final helper = widget.helperText;
    final helperStyle = Theme.of(context).textTheme.bodySmall?.copyWith(
          color: Theme.of(context).colorScheme.onSurfaceVariant,
        );

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        CompositedTransformTarget(
          key: _targetKey,
          link: _layerLink,
          child: InkWell(
            borderRadius: _selectMenuRadius,
            onTap: _toggle,
            child: InputDecorator(
              isEmpty: selected == null,
              decoration: InputDecoration(
                labelText: widget.label,
                prefixIcon: widget.prefixIcon,
                isDense: widget.compact,
                border: widget.compact ? InputBorder.none : null,
                contentPadding: widget.compact ? const EdgeInsets.symmetric(horizontal: 8, vertical: 8) : null,
              ),
              child: Row(
                mainAxisSize: widget.compact ? MainAxisSize.min : MainAxisSize.max,
                children: [
                  Flexible(child: Text(selected?.label ?? '', overflow: TextOverflow.ellipsis)),
                  const SizedBox(width: 4),
                  Icon(_isOpen ? Icons.arrow_drop_up : Icons.arrow_drop_down),
                ],
              ),
            ),
          ),
        ),
        if (!_isOpen && helper != null && helper.isNotEmpty)
          Padding(
            padding: EdgeInsetsDirectional.only(
              start: widget.compact ? 8 : 12,
              top: 4,
            ),
            child: Text(helper, style: helperStyle),
          ),
      ],
    );
  }
}

String formatBytes(num? bytes, {int decimals = 1}) {
  final value = bytes?.toDouble() ?? 0;
  if (value <= 0) return '0 B';
  const suffixes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  final index = math.min((math.log(value) / math.log(1024)).floor(), suffixes.length - 1).toInt();
  return '${(value / math.pow(1024, index)).toStringAsFixed(decimals)} ${suffixes[index]}';
}

String prettyJson(dynamic value) => const JsonEncoder.withIndent('  ').convert(value);

int unixStartOfDay(DateTime date) =>
    DateTime(date.year, date.month, date.day).millisecondsSinceEpoch ~/ 1000;

int unixEndOfDay(DateTime date) =>
    DateTime(date.year, date.month, date.day, 23, 59, 59).millisecondsSinceEpoch ~/ 1000;

String formatTimestamp(dynamic timestamp) {
  final seconds = int.tryParse(timestamp?.toString() ?? '') ?? 0;
  if (seconds <= 0) return '—';
  return DateFormat('yyyy-MM-dd HH:mm:ss').format(
    DateTime.fromMillisecondsSinceEpoch(seconds * 1000),
  );
}

void showMessage(BuildContext context, String message, {bool error = false}) {
  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(
      content: Text(message),
      backgroundColor: error ? Theme.of(context).colorScheme.error : null,
    ),
  );
}

Future<bool> confirm(
  BuildContext context, {
  required String title,
  required String message,
  String action = '',
}) async {
  return await showDialog<bool>(
        context: context,
        builder: (context) => AlertDialog(
          title: Text(title),
          content: Text(message),
          actions: [
            TextButton(onPressed: () => Navigator.pop(context, false), child: Text(context.t('common.cancel'))),
            FilledButton(onPressed: () => Navigator.pop(context, true), child: Text(action.isEmpty ? context.t('common.confirm') : action)),
          ],
        ),
      ) ??
      false;
}

class PageHeader extends StatelessWidget {
  const PageHeader({
    super.key,
    required this.title,
    this.subtitle,
    this.actions = const [],
  });

  final String title;
  final String? subtitle;
  final List<Widget> actions;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(title, style: Theme.of(context).textTheme.headlineSmall?.copyWith(fontWeight: FontWeight.w700)),
                if (subtitle != null) ...[
                  const SizedBox(height: 4),
                  Text(subtitle!, style: Theme.of(context).textTheme.bodyMedium?.copyWith(color: Theme.of(context).colorScheme.onSurfaceVariant)),
                ],
              ],
            ),
          ),
          ...actions,
        ],
      ),
    );
  }
}

class EmptyState extends StatelessWidget {
  const EmptyState({super.key, required this.label, this.icon = Icons.inbox_outlined});
  final String label;
  final IconData icon;

  @override
  Widget build(BuildContext context) => Center(
        child: Padding(
          padding: const EdgeInsets.all(40),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(icon, size: 48, color: Theme.of(context).colorScheme.outline),
              const SizedBox(height: 12),
              Text(label, style: TextStyle(color: Theme.of(context).colorScheme.onSurfaceVariant)),
            ],
          ),
        ),
      );
}

class FilterCard extends StatelessWidget {
  const FilterCard({super.key, required this.child});
  final Widget child;

  @override
  Widget build(BuildContext context) => Card(
        margin: const EdgeInsets.fromLTRB(12, 4, 12, 12),
        child: Padding(padding: const EdgeInsets.all(12), child: child),
      );
}

class JsonEditorDialog extends StatefulWidget {
  const JsonEditorDialog({
    super.key,
    required this.title,
    required this.initialValue,
    required this.onSave,
    this.actionLabel = '',
  });

  final String title;
  final dynamic initialValue;
  final Future<void> Function(dynamic value) onSave;
  final String actionLabel;

  @override
  State<JsonEditorDialog> createState() => _JsonEditorDialogState();
}

class _JsonEditorDialogState extends State<JsonEditorDialog> {
  late final TextEditingController controller;
  bool saving = false;
  String? error;

  @override
  void initState() {
    super.initState();
    controller = TextEditingController(text: prettyJson(widget.initialValue));
  }

  @override
  void dispose() {
    controller.dispose();
    super.dispose();
  }

  Future<void> save() async {
    dynamic value;
    try {
      value = jsonDecode(controller.text);
    } catch (exception) {
      setState(() => error = 'JSON: $exception');
      return;
    }
    setState(() {
      saving = true;
      error = null;
    });
    try {
      await widget.onSave(value);
      if (mounted) Navigator.pop(context, true);
    } catch (exception) {
      if (mounted) setState(() => error = exception.toString());
    } finally {
      if (mounted) setState(() => saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Dialog.fullscreen(
      child: Scaffold(
        appBar: AppBar(
          title: Text(widget.title),
          leading: IconButton(icon: const Icon(Icons.close), onPressed: () => Navigator.pop(context)),
          actions: [
            TextButton.icon(
              onPressed: saving ? null : save,
              icon: saving
                  ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                  : const Icon(Icons.save_outlined),
              label: Text(widget.actionLabel.isEmpty ? context.t('common.save') : widget.actionLabel),
            ),
          ],
        ),
        body: SafeArea(
          child: Padding(
            padding: const EdgeInsets.all(12),
            child: Column(
              children: [
                if (error != null)
                  Padding(
                    padding: const EdgeInsets.only(bottom: 8),
                    child: Text(error!, style: TextStyle(color: Theme.of(context).colorScheme.error)),
                  ),
                Expanded(
                  child: TextField(
                    controller: controller,
                    expands: true,
                    maxLines: null,
                    minLines: null,
                    textAlignVertical: TextAlignVertical.top,
                    autocorrect: false,
                    enableSuggestions: false,
                    style: const TextStyle(fontFamily: 'monospace', fontSize: 13),
                    decoration: const InputDecoration(hintText: '{}', alignLabelWithHint: true),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
