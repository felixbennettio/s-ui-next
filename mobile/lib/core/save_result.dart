class SaveResult {
  const SaveResult({this.refreshFailed = false, this.applied = true});

  final bool refreshFailed;
  final bool applied;

  String get messageKey => refreshFailed
      ? 'common.savedRefreshFailed'
      : applied
          ? 'common.savedApplied'
          : 'common.saved';
}
