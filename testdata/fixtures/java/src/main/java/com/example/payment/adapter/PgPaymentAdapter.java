package com.example.payment.adapter;

import com.example.payment.port.PaymentPort;

public class PgPaymentAdapter implements PaymentPort {

    @Override
    public void charge(long orderId, long amount) {
        // external PG call
    }

    @Override
    public void refund(long orderId) {
        // external PG call
    }
}
