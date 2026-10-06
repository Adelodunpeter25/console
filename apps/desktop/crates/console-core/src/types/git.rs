// Canonical wire types from the shared protobuf schema
// (proto/console/v1/git.proto): status codes stay plain strings, numstat
// counts narrow to u32 so they stay JSON numbers.
//
// The summary counters lived on the hand-written struct as inherent methods;
// generated types are foreign so they move to GitStatusSummaryExt,
// re-exported alongside the types.
pub use console_proto::{
    GitBranchInfo, GitBranchesResponse, GitCheckoutRequest, GitCheckoutResponse, GitDiffResponse,
    GitFileEntry, GitStatusSummary,
};

pub trait GitStatusSummaryExt {
    fn modified_count(&self) -> usize;
    fn staged_count(&self) -> usize;
    fn untracked_count(&self) -> usize;
    fn is_clean(&self) -> bool;
}

impl GitStatusSummaryExt for GitStatusSummary {
    fn modified_count(&self) -> usize {
        self.files
            .iter()
            .filter(|f| f.status == "M" && !f.staged)
            .count()
    }
    fn staged_count(&self) -> usize {
        self.files.iter().filter(|f| f.staged).count()
    }
    fn untracked_count(&self) -> usize {
        self.files.iter().filter(|f| f.status == "?").count()
    }
    fn is_clean(&self) -> bool {
        self.clean
    }
}
