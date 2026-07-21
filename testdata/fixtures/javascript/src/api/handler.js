import svc, { placeOrder } from "../order/service";
import * as check from "../util/check";

export const handleRequest = async (req) => {
  check.validate(req);
  return placeOrder(req, {});
};
