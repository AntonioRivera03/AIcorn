// Route search-param parsing shared by the public pages.
export const optionalString = (value: unknown) =>
  typeof value === "string" && value !== "" ? value : undefined;
