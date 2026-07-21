package com.example.order.adapter.in.web;

import com.example.order.domain.OrderService;
import com.example.order.domain.ProcessRequest;

public class OrderController {

    private final OrderService orderService;

    public OrderController(OrderService orderService) {
        this.orderService = orderService;
    }

    public void handle(ProcessRequest request) {
        orderService.process(request);
    }
}
