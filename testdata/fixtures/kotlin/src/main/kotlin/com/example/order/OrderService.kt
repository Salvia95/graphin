package com.example.order

import com.example.payment.PaymentGateway

class OrderService(private val gateway: PaymentGateway) {

    fun process(request: ProcessRequest): Receipt {
        gateway.charge(request.orderId, request.amount)
        return Receipt(request.orderId)
    }

    fun cancel(orderId: Long, reason: String = "none", notify: Boolean = true): Boolean {
        gateway.refund(orderId)
        if (notify) {
            println("cancelled $orderId: $reason")
        }
        return true
    }
}

data class ProcessRequest(val orderId: Long, val amount: Long)

data class Receipt(val orderId: Long)
