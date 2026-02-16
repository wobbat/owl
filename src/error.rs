//! Error handling utilities

use anyhow::Result;
use std::process;

/// Print an error message and exit with code 1
pub fn exit_with_error(error: anyhow::Error) -> ! {
    eprintln!("{}", crate::internal::color::red(&error.to_string()));
    process::exit(1);
}

/// Handle a Result by printing the error (with operation context) but not exiting
/// Returns true if there was an error
pub fn handle_error_with_context(operation: &str, result: Result<()>) -> bool {
    if let Err(e) = result {
        eprintln!(
            "{}",
            crate::internal::color::red(&format!("Failed to {}: {}", operation, e))
        );
        true
    } else {
        false
    }
}

/// Handle a Result by printing the error (without context) but not exiting
/// Returns true if there was an error
pub fn handle_error(result: Result<()>) -> bool {
    if let Err(e) = result {
        eprintln!("{}", crate::internal::color::red(&e.to_string()));
        true
    } else {
        false
    }
}

/// Handle a Result by printing the error and exiting if failed
pub fn exit_on_error(result: Result<()>) {
    if let Err(e) = result {
        eprintln!("{}", crate::internal::color::red(&e.to_string()));
        process::exit(1);
    }
}
