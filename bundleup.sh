#!/usr/bin/env bash

# Description:
# This script searches for all .go files and go.mod file within a specified directory and its subdirectories,
# excluding the 'bundle' directory itself, the '.git', '.idea', and '.vscode' directories.
# It then empties the 'bundle' directory if it exists (or creates it if it doesn't),
# copies all found .go files and go.mod into the 'bundle' directory,
# and creates a text file with the folder structure and file listing.

# Exit immediately if a command exits with a non-zero status
set -e

# Define the bundle directory name
BUNDLE_DIR="bundle"
# Define the structure file name
STRUCTURE_FILE="$BUNDLE_DIR/files_folders_structure.txt"
# Default root directory is current directory
ROOT_DIR="."

# Parse command line arguments
if [ $# -gt 0 ]; then
    ROOT_DIR="$1"
    # Check if the provided directory exists
    if [ ! -d "$ROOT_DIR" ]; then
        echo "Error: Directory '$ROOT_DIR' does not exist."
        exit 1
    fi
fi

# Function to display messages
echo_msg() {
    echo "[$(date +"%Y-%m-%d %H:%M:%S")] $1"
}

# Step 1: Remove all contents of the bundle directory if it exists, else create it
if [ -d "$BUNDLE_DIR" ]; then
    echo_msg "Emptying the '$BUNDLE_DIR' directory..."
    rm -rf "${BUNDLE_DIR:?}/"*
else
    echo_msg "Creating the '$BUNDLE_DIR' directory..."
    mkdir "$BUNDLE_DIR"
fi

# Step 2: Find all .go files excluding specific directories
echo_msg "Searching for .go files in '$ROOT_DIR'..."
GO_FILES=$(find "$ROOT_DIR" -path "./$BUNDLE_DIR" -prune -o -path "./.git" -prune -o -path "./.idea" -prune -o -path "./.vscode" -prune -o -type f -name "*.go" -print)

# Check if any .go files were found
if [ -z "$GO_FILES" ]; then
    echo_msg "No .go files found."
else
    # Step 3: Copy each .go file to the bundle directory
    echo_msg "Copying .go files to '$BUNDLE_DIR'..."
    while IFS= read -r file; do
        cp "$file" "$BUNDLE_DIR/"
        echo_msg "Copied: $file"
    done <<< "$GO_FILES"
fi

# Step 4: Find and copy go.mod file if it exists
echo_msg "Searching for go.mod file in '$ROOT_DIR'..."
if [ -f "$ROOT_DIR/go.mod" ]; then
    cp "$ROOT_DIR/go.mod" "$BUNDLE_DIR/"
    echo_msg "Copied: $ROOT_DIR/go.mod"
else
    echo_msg "No go.mod file found."
fi

# Step 5: Create a text file with folder structure and file listing
echo_msg "Creating folder structure file..."
{
    echo "Folder Structure:"
    echo "================="
    find "$ROOT_DIR" -path "./$BUNDLE_DIR" -prune -o -path "./.git" -prune -o -path "./.idea" -prune -o -path "./.vscode" -prune -o -type d -print | sort

    echo -e "\nFile Listing:"
    echo "============="
    find "$ROOT_DIR" -path "./$BUNDLE_DIR" -prune -o -path "./.git" -prune -o -path "./.idea" -prune -o -path "./.vscode" -prune -o -type f -print | sort

    echo -e "\nBundled Files:"
    echo "=============="
    ls -la "$BUNDLE_DIR"
} > "$STRUCTURE_FILE"

echo_msg "All files have been bundled successfully from '$ROOT_DIR'."
echo_msg "Folder structure and file listing saved to $STRUCTURE_FILE"