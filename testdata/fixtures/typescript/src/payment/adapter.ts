import { PaymentPort, Status } from "./port";

export function fee(amount: number): number;
export function fee(amount: number, rate: number): number;
export function fee(amount: number, rate: number = 0.1): number {
  return amount * rate;
}

export default class PgAdapter implements PaymentPort {
  private client: HttpClient;

  charge(req: ChargeRequest, retry?: boolean): Receipt {
    const total = fee(req.amount);
    return new Receipt(total, Status.OPEN);
  }

  refund = async (id: string): Promise<void> => {
    await this.client.post(id);
  };
}
