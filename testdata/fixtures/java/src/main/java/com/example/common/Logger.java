package com.example.common;

import java.util.List;

public class Logger {

    public void info(String message) {
        // sink
    }

    public void infoAll(List<String> messages) {
        for (String m : messages) {
            info(m);
        }
    }
}
