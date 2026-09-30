// Server errors are Go-style lowercase sentences; capitalize them for display.
export const sentence = (message: string) =>
  message.charAt(0).toUpperCase() + message.slice(1);
