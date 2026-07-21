package com.example.order.domain;

import com.example.common.Logger;
import com.example.payment.port.PaymentPort;

import java.util.List;

public class OrderService {

    private final PaymentPort paymentPort;
    private final Logger logger;

    public OrderService(PaymentPort paymentPort, Logger logger) {
        this.paymentPort = paymentPort;
        this.logger = logger;
    }

    public Receipt process(ProcessRequest request) {
        return process(request, false);
    }

    public Receipt process(ProcessRequest request, boolean dryRun) {
        logger.info("processing order " + request.orderId());
        if (!dryRun) {
            paymentPort.charge(request.orderId(), request.amount());
        }
        return new Receipt(request.orderId());
    }

    public void cancelPayment(long orderId, String reason) {
        logger.info("cancel payment for " + orderId + ": " + reason);
        paymentPort.refund(orderId);
    }

    public List<Receipt> batch(List<ProcessRequest> requests) {
        return requests.stream().map(this::process).toList();
    }

    public static class Receipt {
        private final long orderId;

        public Receipt(long orderId) {
            this.orderId = orderId;
        }

        public long orderId() {
            return orderId;
        }
    }
}
