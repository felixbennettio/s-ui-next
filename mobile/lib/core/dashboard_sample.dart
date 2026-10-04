import 'dart:math' as math;

class DashboardSample {
  const DashboardSample({required this.time, required this.cpu, required this.sent,
    required this.received, this.uploadRate = 0, this.downloadRate = 0});

  factory DashboardSample.fromStatus(Map<String, dynamic> status, DateTime time, DashboardSample? previous) {
    final net = status['net'] is Map ? status['net'] as Map : const {};
    final sent = (net['sent'] as num?)?.toDouble() ?? 0;
    final received = (net['recv'] as num?)?.toDouble() ?? 0;
    final elapsed = previous == null ? 0 : time.difference(previous.time).inMilliseconds / 1000;
    double rate(double current, double? before) => elapsed > 0 && before != null
        ? math.max(0, current - before) / elapsed : 0;
    return DashboardSample(time: time, cpu: (status['cpu'] as num?)?.toDouble() ?? 0,
      sent: sent, received: received, uploadRate: rate(sent, previous?.sent),
      downloadRate: rate(received, previous?.received));
  }

  final DateTime time;
  final double cpu, sent, received, uploadRate, downloadRate;
}
