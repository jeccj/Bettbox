import 'dart:async';

import 'package:bett_box/clash/clash.dart';
import 'package:bett_box/common/common.dart';
import 'package:bett_box/models/models.dart';
import 'package:bett_box/state.dart';
import 'package:bett_box/widgets/dialog.dart';
import 'package:flutter/material.dart';

Future<void> showAppTrafficDialog(BuildContext context) async {
  await globalState.showCommonDialog<void>(child: const AppTrafficDialog());
}

class AppTrafficDialog extends StatefulWidget {
  const AppTrafficDialog({super.key});

  @override
  State<AppTrafficDialog> createState() => _AppTrafficDialogState();
}

class _AppTrafficDialogState extends State<AppTrafficDialog> {
  List<AppTraffic> _items = const [];
  Timer? _timer;
  bool _fetching = false;
  bool _loaded = false;

  @override
  void initState() {
    super.initState();
    _fetch();
    _timer = Timer.periodic(const Duration(seconds: 2), (_) {
      _fetch();
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  Future<void> _fetch() async {
    if (!mounted || _fetching) return;
    _fetching = true;
    try {
      final items = await clashCore.getAppTraffic();
      if (!mounted) return;
      setState(() {
        _items = items;
        _loaded = true;
      });
    } catch (_) {
      if (mounted) {
        setState(() {
          _loaded = true;
        });
      }
    } finally {
      _fetching = false;
    }
  }

  String _formatBytes(int bytes) {
    return TrafficValue(value: bytes).shortShow;
  }

  Widget _buildValueCell(String value, {bool emphasized = false}) {
    return SizedBox(
      width: 64,
      child: Text(
        value,
        maxLines: 1,
        overflow: TextOverflow.ellipsis,
        textAlign: TextAlign.end,
        style: context.textTheme.bodySmall?.copyWith(
          fontWeight: emphasized ? FontWeight.w600 : null,
        ),
      ),
    );
  }

  Widget _buildHeader(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: Row(
        children: [
          Expanded(
            child: Text(
              appLocalizations.application,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: context.textTheme.labelSmall?.copyWith(
                color: context.colorScheme.onSurfaceVariant,
              ),
            ),
          ),
          _buildValueCell(appLocalizations.upload),
          _buildValueCell(appLocalizations.download),
          _buildValueCell(appLocalizations.totalTraffic),
        ],
      ),
    );
  }

  Widget _buildRow(AppTraffic item) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 5),
      child: Row(
        children: [
          Expanded(
            child: Text(
              item.process,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
              style: context.textTheme.bodySmall,
            ),
          ),
          _buildValueCell(_formatBytes(item.upload)),
          _buildValueCell(_formatBytes(item.download)),
          _buildValueCell(_formatBytes(item.total), emphasized: true),
        ],
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return CommonDialog(
      title: appLocalizations.trafficUsage,
      padding: const EdgeInsets.fromLTRB(16, 14, 16, 8),
      actions: [
        TextButton(
          onPressed: () {
            Navigator.of(context, rootNavigator: true).pop();
          },
          child: Text(appLocalizations.confirm),
        ),
      ],
      child: !_loaded && _items.isEmpty
          ? const Center(child: CircularProgressIndicator())
          : Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                _buildHeader(context),
                if (_items.isEmpty)
                  const Padding(
                    padding: EdgeInsets.symmetric(vertical: 16),
                    child: Center(child: Text('—')),
                  )
                else
                  for (final item in _items) _buildRow(item),
              ],
            ),
    );
  }
}
