use anyhow::{Result, anyhow};
use std::io::{self, Read, Write};
use std::process::{Command, Stdio};
use std::sync::{Arc, Mutex, mpsc};
use std::thread;
use std::time::Duration;

/// Spinner display functionality
pub mod spinner {
    use super::*;

    // Shared spinner frames so all spinners look consistent
    const SPINNER_FRAMES: &[&str] = &["⁚", "⁖", "⁘", "⁛", "⁙", "⁛", "⁘", "⁖"];

    /// Print a spinner frame with message
    pub fn print_frame(message: &str, frame_index: usize) {
        print!(
            "\r\x1b[2K  {} {}...",
            crate::internal::color::blue(SPINNER_FRAMES[frame_index % SPINNER_FRAMES.len()]),
            message
        );
        io::stdout().flush().ok();
    }

    /// Clear the current spinner line
    pub fn clear_line() {
        print!("\r\x1b[2K");
        io::stdout().flush().ok();
    }

    /// Configuration for spinner behavior
    pub struct SpinnerConfig {
        pub timeout_secs: u64,
        pub delay_ms: u64,
        pub cleanup_on_timeout: Option<Box<dyn FnOnce() + Send>>,
    }

    impl Default for SpinnerConfig {
        fn default() -> Self {
            Self {
                timeout_secs: 30 * 60, // 30 minutes
                delay_ms: crate::internal::constants::SPINNER_DELAY_MS,
                cleanup_on_timeout: None,
            }
        }
    }

    impl SpinnerConfig {
        pub fn with_cleanup<F>(mut self, cleanup: F) -> Self
        where
            F: FnOnce() + Send + 'static,
        {
            self.cleanup_on_timeout = Some(Box::new(cleanup));
            self
        }
    }
}

/// Command execution functionality
pub mod command {
    use super::*;

    /// Common setup for command execution with pipes
    pub struct CommandSetup {
        pub child: Arc<Mutex<std::process::Child>>,
        pub stdout: Option<std::process::ChildStdout>,
        pub stderr: Option<std::process::ChildStderr>,
    }

    impl CommandSetup {
        pub fn new(command: &str, args: &[&str]) -> anyhow::Result<Self> {
            let mut cmd = Command::new(command);
            cmd.args(args)
                .stdin(Stdio::inherit())
                .stdout(Stdio::piped())
                .stderr(Stdio::piped());

            let mut child = cmd
                .spawn()
                .map_err(|e| anyhow!("Failed to spawn {}: {}", command, e))?;

            let stdout = child.stdout.take();
            let stderr = child.stderr.take();

            Ok(CommandSetup {
                child: Arc::new(Mutex::new(child)),
                stdout,
                stderr,
            })
        }
    }
}

/// Run a spinner with common timeout and animation logic
fn run_with_spinner_common<T, F, C>(
    config: spinner::SpinnerConfig,
    status_getter: F,
    completion_checker: C,
) -> anyhow::Result<T>
where
    F: Fn() -> String,
    C: Fn() -> anyhow::Result<Option<anyhow::Result<T>>>,
{
    let mut i = 0;
    let start_time = std::time::Instant::now();
    let timeout_duration = Duration::from_secs(config.timeout_secs);

    loop {
        let current_msg = status_getter();
        spinner::print_frame(&current_msg, i);

        // Check for timeout
        if start_time.elapsed() > timeout_duration {
            spinner::clear_line();
            if let Some(cleanup) = config.cleanup_on_timeout {
                cleanup();
            }
            return Err(anyhow!(
                "Operation timed out after {} minutes",
                config.timeout_secs / 60
            ));
        }

        // Check if operation is complete
        match completion_checker() {
            Ok(Some(result)) => {
                spinner::clear_line();
                return result;
            }
            Ok(None) => {
                // Still running, continue
                thread::sleep(Duration::from_millis(config.delay_ms));
                i += 1;
            }
            Err(e) => {
                spinner::clear_line();
                return Err(e);
            }
        }
    }
}

/// Execute a command with spinner progress display
pub fn execute_command_with_spinner(
    command: &str,
    args: &[&str],
    message: &str,
) -> anyhow::Result<std::process::ExitStatus> {
    let setup = command::CommandSetup::new(command, args)?;

    // Get stdout handle for reading output
    let stdout = setup
        .stdout
        .ok_or_else(|| anyhow!("Failed to get child stdout"))?;
    let stderr = setup
        .stderr
        .ok_or_else(|| anyhow!("Failed to get child stderr"))?;
    let current_status = Arc::new(Mutex::new(message.to_string()));

    // Start threads to read and parse output
    start_output_reader(stdout, Arc::clone(&current_status));
    start_output_reader(stderr, Arc::clone(&current_status));

    let child_clone = Arc::clone(&setup.child);
    run_with_spinner_common(
        spinner::SpinnerConfig::default().with_cleanup(move || {
            if let Ok(mut child_guard) = child_clone.lock() {
                let _ = child_guard.kill();
            }
        }),
        || match current_status.lock() {
            Ok(guard) => guard.clone(),
            Err(poisoned) => poisoned.into_inner().clone(),
        },
        || match setup.child.lock().unwrap().try_wait() {
            Ok(Some(status)) => Ok(Some(Ok(status))),
            Ok(None) => Ok(None),
            Err(e) => Err(anyhow!("Failed to wait for command: {}", e)),
        },
    )
}

/// Execute a command with spinner and capture stderr for diagnostics
pub fn execute_command_with_stderr_capture(
    command: &str,
    args: &[&str],
    message: &str,
) -> anyhow::Result<(std::process::ExitStatus, String)> {
    let setup = command::CommandSetup::new(command, args)?;

    // Take stdout/stderr for reading
    let stdout = setup
        .stdout
        .ok_or_else(|| anyhow!("Failed to get child stdout"))?;
    let stderr = setup
        .stderr
        .ok_or_else(|| anyhow!("Failed to get child stderr"))?;

    let current_status = Arc::new(Mutex::new(message.to_string()));
    let captured_stderr = Arc::new(Mutex::new(String::new()));

    // Start readers
    start_output_reader(stdout, Arc::clone(&current_status));

    // Capture stderr fully for diagnostics
    {
        let captured_stderr = Arc::clone(&captured_stderr);
        thread::spawn(move || {
            use std::io::{BufRead, BufReader};
            let reader = BufReader::new(stderr);
            for line in reader.lines().map_while(Result::ok) {
                match captured_stderr.lock() {
                    Ok(mut buf) => {
                        buf.push_str(&line);
                        buf.push('\n');
                    }
                    Err(poisoned) => {
                        // If mutex is poisoned, try to recover
                        let mut buf = poisoned.into_inner();
                        buf.push_str(&line);
                        buf.push('\n');
                    }
                }
            }
        });
    }

    let child_clone = Arc::clone(&setup.child);
    let exit_status = run_with_spinner_common(
        spinner::SpinnerConfig::default().with_cleanup(move || {
            if let Ok(mut child_guard) = child_clone.lock() {
                let _ = child_guard.kill();
            }
        }),
        || match current_status.lock() {
            Ok(guard) => guard.clone(),
            Err(poisoned) => poisoned.into_inner().clone(),
        },
        || match setup.child.lock().unwrap().try_wait() {
            Ok(Some(status)) => Ok(Some(Ok(status))),
            Ok(None) => Ok(None),
            Err(e) => Err(anyhow!("Failed to wait for command: {}", e)),
        },
    )?;

    let stderr_output = match captured_stderr.lock() {
        Ok(guard) => guard.clone(),
        Err(poisoned) => {
            // If mutex is poisoned, use the poisoned value
            poisoned.into_inner().clone()
        }
    };
    Ok((exit_status, stderr_output))
}

/// Execute a command interactively, inheriting terminal stdin/stdout/stderr.
/// This allows child processes to prompt for input (e.g. y/n questions).
pub fn execute_command_interactive(
    command: &str,
    args: &[&str],
    message: &str,
) -> anyhow::Result<std::process::ExitStatus> {
    println!("  {} {}", crate::internal::color::blue("info:"), message);
    Command::new(command)
        .args(args)
        .stdin(Stdio::inherit())
        .stdout(Stdio::inherit())
        .stderr(Stdio::inherit())
        .status()
        .map_err(|e| anyhow!("Failed to run {}: {}", command, e))
}

/// Execute a command with retry logic and spinner progress display
pub fn execute_command_with_retry(
    command: &str,
    args: &[String],
    base_message: &str,
    max_retries: usize,
) -> anyhow::Result<std::process::ExitStatus> {
    let mut last_error = None;

    for attempt in 0..=max_retries {
        // Create a channel for spinner status updates
        let (status_tx, _status_rx) = mpsc::channel();

        // Start the command with spinner in a separate thread
        let command_thread = {
            let command = command.to_string();
            let args = args.to_vec();
            let base_message = base_message.to_string();
            let status_tx = status_tx.clone();

            thread::spawn(move || {
                execute_command_with_dynamic_spinner(
                    &command,
                    &args,
                    &base_message,
                    attempt,
                    max_retries,
                    status_tx,
                )
            })
        };

        // Monitor the command and handle retries
        match command_thread.join() {
            Ok(Ok(status)) => return Ok(status),
            Ok(Err(err)) => {
                last_error = Some(err);

                // Check if this is a network-related error that we should retry
                let err_msg = last_error.as_ref().unwrap().to_string();
                let should_retry = err_msg.contains("Connection reset by peer")
                    || err_msg.contains("error sending request")
                    || err_msg.contains("error trying to connect")
                    || err_msg.contains("os error 104");

                if !should_retry || attempt == max_retries {
                    return Err(last_error.unwrap());
                }

                // Exponential backoff: 1s, 2s, 4s, 8s, 16s
                let delay = Duration::from_secs(1 << attempt);

                // Update spinner message to show retry status
                let retry_message = format!(
                    "Retrying due to network errors... ({}/{})",
                    attempt + 1,
                    max_retries + 1
                );
                spinner::clear_line();
                print!("{}", retry_message);
                std::io::stdout().flush().ok();

                // Sleep for the retry delay
                thread::sleep(delay);

                // Clear the line for the next spinner iteration
                spinner::clear_line();
            }
            Err(_) => {
                return Err(anyhow!("Command thread panicked"));
            }
        }
    }

    Err(last_error.unwrap_or_else(|| anyhow!("Unknown error")))
}

/// Internal function to run command with dynamic spinner updates
fn execute_command_with_dynamic_spinner(
    command: &str,
    args: &[String],
    base_message: &str,
    attempt: usize,
    max_retries: usize,
    _status_tx: mpsc::Sender<String>,
) -> anyhow::Result<std::process::ExitStatus> {
    let setup = command::CommandSetup::new(
        command,
        &args.iter().map(|s| s.as_str()).collect::<Vec<_>>(),
    )?;

    // Get stdout handle for reading output
    let stdout = setup
        .stdout
        .ok_or_else(|| anyhow!("Failed to get child stdout"))?;
    let stderr = setup
        .stderr
        .ok_or_else(|| anyhow!("Failed to get child stderr"))?;
    let current_status = Arc::new(Mutex::new(base_message.to_string()));

    // Start threads to read and parse output
    start_output_reader(stdout, Arc::clone(&current_status));
    start_output_reader(stderr, Arc::clone(&current_status));

    let child_clone = Arc::clone(&setup.child);
    run_with_spinner_common(
        spinner::SpinnerConfig::default().with_cleanup(move || {
            if let Ok(mut child_guard) = child_clone.lock() {
                let _ = child_guard.kill();
            }
        }),
        || {
            let base_msg = match current_status.lock() {
                Ok(guard) => guard.clone(),
                Err(poisoned) => poisoned.into_inner().clone(),
            };
            if attempt > 0 {
                format!("{} (retry {}/{})", base_msg, attempt, max_retries + 1)
            } else {
                base_msg
            }
        },
        || match setup.child.lock().unwrap().try_wait() {
            Ok(Some(status)) => Ok(Some(Ok(status))),
            Ok(None) => Ok(None),
            Err(e) => Err(anyhow!("Failed to wait for command: {}", e)),
        },
    )
}

/// Execute an operation with spinner progress display
pub fn execute_with_progress<T, F>(operation: F, message: &str) -> anyhow::Result<T>
where
    F: FnOnce() -> anyhow::Result<T> + Send + 'static,
    T: Send + 'static,
{
    // Channel to communicate result from operation thread
    let (tx, rx) = std::sync::mpsc::channel();

    // Spawn thread for the operation
    thread::spawn(move || {
        let result = operation();
        let _ = tx.send(result);
    });

    run_with_spinner_common(
        spinner::SpinnerConfig::default(),
        || message.to_string(),
        || match rx.try_recv() {
            Ok(result) => Ok(Some(result)),
            Err(std::sync::mpsc::TryRecvError::Empty) => Ok(None),
            Err(std::sync::mpsc::TryRecvError::Disconnected) => {
                Err(anyhow!("Operation thread ended unexpectedly"))
            }
        },
    )
}

fn update_spinner_status(status: &Arc<Mutex<String>>, status_msg: String) {
    match status.lock() {
        Ok(mut guard) => *guard = status_msg,
        Err(poisoned) => {
            // If mutex is poisoned, try to recover
            *poisoned.into_inner() = status_msg;
        }
    }
}

fn is_prompt_like(text: &str) -> bool {
    let lower = text.to_ascii_lowercase();
    lower.contains("[y/n]")
        || lower.contains("(y/n)")
        || lower.contains("proceed with installation?")
        || lower.contains("continue?")
}

fn parse_status_message(line: &str) -> Option<String> {
    let line = line.trim();
    if line.is_empty() {
        return None;
    }

    if is_prompt_like(line) {
        return Some(line.to_string());
    }

    if line.starts_with("::") {
        return None;
    }

    let status_msg = if let Some(pkg) = extract_package_name(line) {
        if line.contains("upgrading") {
            format!("Upgrading {}", pkg)
        } else if line.contains("installing") {
            format!("Installing {}", pkg)
        } else {
            line.to_string()
        }
    } else {
        line.to_string()
    };
    Some(status_msg)
}

fn start_output_reader<R>(stream: R, status: Arc<Mutex<String>>)
where
    R: Read + Send + 'static,
{
    thread::spawn(move || {
        let mut reader = io::BufReader::new(stream);
        let mut chunk = [0u8; 1024];
        let mut pending = String::new();

        loop {
            let bytes_read = match reader.read(&mut chunk) {
                Ok(0) => break,
                Ok(n) => n,
                Err(_) => break,
            };

            pending.push_str(&String::from_utf8_lossy(&chunk[..bytes_read]));

            while let Some(pos) = pending.find(|c| c == '\n' || c == '\r') {
                let line = pending[..pos].to_string();
                let mut drain_len = pos + 1;
                while drain_len < pending.len() {
                    let b = pending.as_bytes()[drain_len];
                    if b == b'\n' || b == b'\r' {
                        drain_len += 1;
                    } else {
                        break;
                    }
                }
                pending.drain(..drain_len);

                if let Some(status_msg) = parse_status_message(&line) {
                    update_spinner_status(&status, status_msg);
                }
            }

            // Some prompts are not newline-terminated (e.g. "[Y/n]").
            if is_prompt_like(&pending) {
                let prompt = pending.trim();
                if !prompt.is_empty() {
                    update_spinner_status(&status, prompt.to_string());
                }
            }
        }

        if let Some(status_msg) = parse_status_message(&pending) {
            update_spinner_status(&status, status_msg);
        }
    });
}

/// Extract package name from common paru/pacman output patterns
fn extract_package_name(line: &str) -> Option<String> {
    // Try parentheses pattern first
    if let Some(pkg_part) = line.split('(').nth(1)?.split(')').next() {
        return pkg_part.split('-').next().map(|s| s.to_string());
    }

    // Fallback for upgrading/installing lines
    if line.contains("upgrading") || line.contains("installing") {
        return line
            .split_whitespace()
            .find(|word| {
                word.contains('-') && word.chars().all(|c| c.is_alphanumeric() || c == '-')
            })
            .and_then(|word| word.split('-').next())
            .map(|s| s.to_string());
    }

    None
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_run_with_spinner() {
        use super::spinner;
        let config = spinner::SpinnerConfig {
            timeout_secs: 1,
            delay_ms: 100,
            cleanup_on_timeout: None,
        };
        let result = run_with_spinner_common(
            config,
            || "Testing spinner".to_string(),
            || Ok(Some(Ok(42))),
        );
        assert_eq!(result.unwrap(), 42);
    }
}
