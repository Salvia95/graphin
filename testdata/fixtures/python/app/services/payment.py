class PaymentClient:
    def charge(self, order_id, amount):
        pass

    def refund(self, order_id):
        pass


def retry(op):
    return op()


def retry(op, attempts):  # same-name redefinition: exercises the #n suffix rule
    for _ in range(attempts):
        op()
