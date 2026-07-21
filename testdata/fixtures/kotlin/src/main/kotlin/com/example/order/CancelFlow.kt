package com.example.order

import com.example.util.toWon

class CancelFlow(private val service: OrderService) {

    fun run(orderId: Long) {
        service.cancel(orderId) // 1 argument against arity range 1..3
        val fee = 100L.toWon()
        println(fee)
    }
}
