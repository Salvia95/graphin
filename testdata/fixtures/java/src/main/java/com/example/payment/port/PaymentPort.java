package com.example.payment.port;

public interface PaymentPort {
    void charge(long orderId, long amount);

    void refund(long orderId);
}
