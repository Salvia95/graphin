import Big from "big.js";
import { validate } from "../util/check";

const repo = require("./repo");

export class OrderService extends BaseService {
  total = 0;

  handle(req) {
    validate(req);
    this.process(req, true);
    return new Receipt(req.id);
  }

  process(req, dryRun = false) {
    const normalize = (r) => repo.save(r);
    return normalize(req);
  }

  static of(cfg) {
    return new OrderService();
  }

  onDone = (e) => {
    this.handle(e);
  };
}

export function placeOrder(req, opts = {}, ...tags) {
  const svc = OrderService.of(opts);
  return svc.handle(req);
}

export default function () {
  return placeOrder(null);
}
