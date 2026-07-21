export interface PaymentPort extends Auditable {
  charge(req: ChargeRequest, retry?: boolean): Receipt;
}

export type PortAlias = PaymentPort;

export enum Status {
  OPEN,
  CLOSED,
}
