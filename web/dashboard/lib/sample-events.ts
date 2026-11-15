export const sampleEvents = [
  {
    id: "order",
    label: "Order created",
    type: "order.created",
    description: "A new order is ready for fulfillment.",
    data: { order_id: "ord_demo_1042", customer_id: "cus_demo_482", total_cents: 4999, currency: "USD" },
  },
  {
    id: "payment",
    label: "Payment succeeded",
    type: "payment.succeeded",
    description: "A payment has been confirmed.",
    data: { payment_id: "pay_demo_2048", order_id: "ord_demo_1042", amount_cents: 4999, currency: "USD" },
  },
  {
    id: "subscription",
    label: "Subscription renewed",
    type: "subscription.renewed",
    description: "A recurring plan has renewed.",
    data: { subscription_id: "sub_demo_1032", customer_id: "cus_demo_482", plan: "pro", billing_period: "monthly" },
  },
] as const;
