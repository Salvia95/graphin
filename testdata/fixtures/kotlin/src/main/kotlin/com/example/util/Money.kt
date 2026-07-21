package com.example.util

data class Money(val won: Long)

fun Long.toWon(): Money = Money(this)

fun sum(values: List<Money>): Money = Money(values.sumOf { it.won })
