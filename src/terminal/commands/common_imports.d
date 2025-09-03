module terminal.commands.common_imports;

// Standard library imports
public import std.algorithm;
public import std.array;
public import std.process;
public import std.stdio;
public import std.string;
public import std.path;
public import std.file;
public import std.format;
public import std.conv;
public import std.algorithm.sorting;

// Terminal module imports
public import terminal.args;
public import terminal.options;
public import terminal.ui;
public import terminal.colors;
public import terminal.prompt;
public import terminal.apply;

// Config module imports
public import config.loader;
public import config.parser;
public import config.paths;
public import config.write;

// Utils module imports
public import utils.process;
public import utils.common;
public import utils.selection;
public import utils.sh;

// Package module imports
public import packages.packages;
public import packages.pacman;
public import packages.aur;
public import packages.types;
public import packages.state;
public import packages.pkgbuild;

// System module imports
public import systems.dotfiles;
public import systems.env;
public import systems.setup;
public import systems.services;
