from app.services.payment import PaymentClient


class OrderService:
    def __init__(self, client: PaymentClient):
        self.client = client

    def process(self, request):
        self.client.charge(request["order_id"], request["amount"])
        return {"order_id": request["order_id"]}

    def cancel(self, order_id, *args, **kwargs):
        self.client.refund(order_id)
        return True


def audit(*events):
    for event in events:
        print(event)
