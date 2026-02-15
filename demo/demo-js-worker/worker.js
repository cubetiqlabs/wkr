function main(req) {
  const tz = new Date().toLocaleTimeString("en-US", { timeZone: "UTC" });
  const name = req.query.name || "World";
  return { message: "Hello from Cubis Workers!", time: tz, random: Math.random(), name, query: req.query };
}
