import React from "react";

export const Widget = ({ title }: WidgetProps) => {
  return <div onClick={() => report(title)}>{title}</div>;
};

export default function Page() {
  return <Widget title="home" />;
}
