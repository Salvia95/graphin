#!/usr/bin/env python3
"""Generates HF-reference token-ID fixtures for internal/tokenizer parity
tests (§7-P4-①). Run offline once per tokenizer revision:

    python3 gen_tokenizer_fixtures.py <tokenizer.json> <out.json>

The Go tests compare their Encode() output against these IDs exactly.
"""
import json
import sys

from tokenizers import Tokenizer

SENTENCES = [
    # English / natural language
    "payment cancellation logic",
    "find the entry point for order processing",
    "How does the refund flow work?",
    "hybrid search with reciprocal rank fusion",
    # Korean / CJK
    "결제 취소 로직",
    "주문 처리의 진입점을 찾아줘",
    "환불 흐름이 어떻게 동작하나요?",
    "하이브리드 검색과 그래프 탐색",
    "한글과 English가 섞인 mixed 문장",
    "決済キャンセル処理",
    "支付取消逻辑",
    # code identifiers & signatures
    "OrderService.process(ProcessRequest)",
    "cancelPayment(long orderId, String reason)",
    "com.example.payment.port.PaymentPort",
    "fun cancel(orderId: Long, reason: String = \"none\")",
    "def retry(op, attempts): pass",
    "snake_case_name camelCaseName HTTPServer2",
    "List<ProcessRequest> batch requests",
    "@Override public void charge(long orderId, long amount)",
    "self.client.refund(order_id)",
    # synthesized summaries (the actual passage shape)
    "method OrderService.process in com.example.order.domain: process ProcessRequest boolean",
    "class PaymentPort in com.example.payment.port: interface charge refund",
    "function toWon in com.example.util: Long extension money",
    "passage: method cancel payment refund order",
    "query: 결제 취소는 어디서 처리해?",
    # edge cases
    "a",
    "x " * 40,
    "über café naïve résumé",
    "1234567890 3.14159 0xDEADBEEF",
    "!!! ??? ... ,,, ///",
]


def main() -> None:
    tok_path, out_path = sys.argv[1], sys.argv[2]
    tok = Tokenizer.from_file(tok_path)
    cases = []
    for s in SENTENCES:
        enc = tok.encode(s)
        cases.append({"text": s, "ids": enc.ids})
    with open(out_path, "w", encoding="utf-8") as f:
        json.dump({"tokenizer": tok_path.split("/")[-1], "cases": cases},
                  f, ensure_ascii=False, indent=1)
    print(f"{out_path}: {len(cases)} cases")


if __name__ == "__main__":
    main()
