import Stripe from "stripe";

let stripeInstance: Stripe | null = null;

export function useStripe() {
  if (!stripeInstance) {
    const config = useRuntimeConfig();
    stripeInstance = new Stripe(config.stripe.secretKey, {
      apiVersion: "2025-09-30.clover",
    });
  }
  return stripeInstance;
}
