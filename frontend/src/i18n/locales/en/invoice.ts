export default {
  invoice: {
    tabs: { apply: 'Apply for Invoice', history: 'My Invoices' },
    selectOrders: {
      title: 'Select Orders', minimum: 'Selected orders must total at least {amount}', selectOrder: 'Select order #{id}',
      orderId: 'Order ID', orderNo: 'Order No.', orderType: 'Order Type', amount: 'Invoice Amount', paidAt: 'Paid At',
      empty: 'There are no eligible orders. Refunded, applied, and invoiced orders cannot be selected again.',
    },
    summary: { selected: '{count} order(s) selected, total {amount}', met: 'The minimum invoice amount is met.', remaining: '{amount} more is required before submitting.' },
    confirm: {
      title: 'Confirm Invoice Details', total: '{count} order(s), invoice amount {amount}',
      warning: 'An invoice application cannot be cancelled by the user after submission. Selected orders cannot be used for another invoice application. Contact an administrator if any information is incorrect.',
      acknowledge: 'I confirm the orders and invoice details are correct and understand this application cannot be cancelled by me.', submit: 'Submit Application',
    },
    header: {
      add: 'Add Header', edit: 'Edit Header', manage: 'Manage Headers', empty: 'No saved invoice headers.', choose: 'Invoice Header', choosePlaceholder: 'Select an invoice header', required: 'Add and select an invoice header first.',
      type: 'Header Type', personal: 'Individual', company: 'Company', title: 'Invoice Title', taxNumber: 'Tax Number', email: 'Email', phone: 'Phone', address: 'Address', default: 'Set as default',
      deleteConfirm: 'Delete invoice header "{title}"? Historical applications will not be changed.',
    },
    history: {
      title: 'Invoice History', subtitle: 'Track offline invoice processing and historical orders.', applicationNo: 'Application No.', amount: 'Invoice Amount', status: 'Status', createdAt: 'Applied At',
      rejectionReason: 'Rejection Reason', invoiceNumber: 'Invoice Number', orderNo: 'Order No.', orderType: 'Order Type',
    },
    admin: {
      searchPlaceholder: 'Search application, order, or user email', export: 'Export filtered applications', minimumAmount: 'Minimum Invoice Amount', allStatuses: 'All Statuses',
      startDate: 'Start Date', endDate: 'End Date', user: 'User', processStatus: 'Processing Status', note: 'Administrator Note',
    },
    status: { PENDING: 'Pending', PROCESSING: 'Processing', INVOICED: 'Invoiced', REJECTED: 'Rejected' },
    orderTypes: { balance: 'Balance Recharge', subscription: 'Subscription', daily_limit_reset: 'Daily Limit Reset' },
    messages: { submitted: 'Invoice application submitted. An administrator will process it offline.' },
    errors: {},
  },
}
