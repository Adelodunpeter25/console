#!/usr/bin/env node
/**
 * Console Agent CLI - Daemon Management Interface
 * 
 * Commands:
 *   console start   - Start the daemon server
 *   console stop    - Stop the running daemon
 *   console status  - Check daemon status
 *   console logs    - Tail daemon logs
 *   console restart - Restart the daemon
 */
import { Command } from "commander";
import { startDaemon } from "./commands/start.js";
import { stopDaemon } from "./commands/stop.js";
import { statusDaemon } from "./commands/status.js";
import { logsDaemon } from "./commands/logs.js";
import { restartDaemon } from "./commands/restart.js";

const program = new Command();

program
  .name("console")
  .description("Console Agent - AI coding agent daemon")
  .version("1.0.0");

program
  .command("start")
  .description("Start the console agent daemon")
  .option("-p, --port <number>", "Port to run the server on", "3000")
  .option("-h, --host <string>", "Host to bind to", "0.0.0.0")
  .option("-d, --daemon", "Run as background daemon", true)
  .option("--no-daemon", "Run in foreground")
  .action(startDaemon);

program
  .command("stop")
  .description("Stop the running console agent daemon")
  .action(stopDaemon);

program
  .command("status")
  .description("Check the status of the console agent daemon")
  .action(statusDaemon);

program
  .command("logs")
  .description("Show console agent daemon logs")
  .option("-f, --follow", "Follow log output (tail -f)")
  .option("-n, --lines <number>", "Number of lines to show", "50")
  .action(logsDaemon);

program
  .command("restart")
  .description("Restart the console agent daemon")
  .option("-p, --port <number>", "Port to run the server on", "3000")
  .option("-h, --host <string>", "Host to bind to", "0.0.0.0")
  .action(restartDaemon);

program.parse();