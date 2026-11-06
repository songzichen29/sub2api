export default {
  monitorDisplay: {
    title: 'V1 user display settings',
    description: 'Move categories and channels using the arrows. Saved order applies when the user status page refreshes. Link a billing group to show its current token multiplier when available to the viewer. These settings do not change billing prices.',
    moveUp: 'Move {name} up',
    moveDown: 'Move {name} down',
    billingGroup: 'Linked billing group',
    unlinked: 'Unlinked (hide multiplier)',
    missingGroup: 'Previous group #{id} no longer exists; select another',
    disabled: 'Disabled',
    noMonitors: 'No monitors in this category; empty categories are hidden from users',
    loadFailed: 'Failed to load display settings',
    saveFailed: 'Failed to save display settings',
    saved: 'Display settings saved; changes apply on the next user status refresh',
    currentRate: 'Current token multiplier {rate}×',
    rateHint: 'Uses the linked billing group, including your custom rate and the current peak factor. Separately priced images, videos and other products follow their billing settings.',
  },
}
