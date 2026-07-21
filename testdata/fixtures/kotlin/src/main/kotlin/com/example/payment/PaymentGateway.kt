package com.example.payment

interface PaymentGateway {
    fun charge(orderId: Long, amount: Long)

    fun refund(orderId: Long)
}

class PgGateway : PaymentGateway {

    override fun charge(orderId: Long, amount: Long) {
        // external call
    }

    override fun refund(orderId: Long) {
        // external call
    }
}
