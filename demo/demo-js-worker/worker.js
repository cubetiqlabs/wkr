function main(req) {
  const tz = new Date().toLocaleTimeString("en-US", { timeZone: "UTC" });
  return { message: "Hello from Cubis Workers!", time: tz, random: Math.random(), req: req };
}
