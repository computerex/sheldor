#!/usr/bin/env python3
"""
1G1R (1 Game 1 ROM) Filter Script - Language-Based Edition
Filters a game list to keep only one ROM per game, organized by language.
"""

import json
import re
import argparse
from collections import defaultdict
from typing import List, Dict, Any, Tuple


def extract_base_name_and_region(game_name: str) -> Tuple[str, str, str]:
    """
    Extract the base game name, region, and disc info from the full game name.
    
    Returns: (base_name, region, disc_info)
    
    Examples:
        "007 - Blood Stone (Europe).zip" -> ("007 - Blood Stone", "Europe", "")
        "Final Fantasy VII (USA) (Disc 1).chd" -> ("Final Fantasy VII", "USA", "Disc 1")
        "Final Fantasy VII (USA) (Disc 2).chd" -> ("Final Fantasy VII", "USA", "Disc 2")
        "Pokemon - White Version (USA, Europe).zip" -> ("Pokemon - White Version", "USA, Europe", "")
    """
    # Remove file extension
    name_without_ext = re.sub(r'\.(zip|7z|rar|nds|chd|cue|bin|iso|rvz)$', '', game_name, flags=re.IGNORECASE)
    
    # Extract disc info if present (Disc 1, Disc 2, etc.)
    disc_info = ""
    disc_pattern = r'\s*\(Disc\s*\d+\)'
    disc_match = re.search(disc_pattern, name_without_ext, re.IGNORECASE)
    if disc_match:
        disc_info = disc_match.group(0).strip()
        # Remove leading/trailing parens and whitespace for clean disc info
        disc_info = disc_info.strip('() ')
        # Remove disc info from the name for base name extraction
        name_without_ext = name_without_ext[:disc_match.start()] + name_without_ext[disc_match.end():]
        name_without_ext = name_without_ext.strip()
    
    # Look for region in parentheses at the end (before any language tags)
    # Pattern: (Region) or (Region) (Language)
    region_pattern = r'\(([^)]+)\)(?:\s*\([^)]*\))*$'
    
    match = re.search(region_pattern, name_without_ext)
    if match:
        region_text = match.group(1)
        base_name = name_without_ext[:match.start()].strip()
        
        # Check if it's a language code (En,Fr,De etc) vs multi-region (USA, Europe)
        # Language codes have 2-letter codes like "En", "Fr", "De", "Es", "It", "Ja"
        is_language_code = False
        if ',' in region_text:
            # Check if it contains language codes (2 letters)
            parts = [p.strip() for p in region_text.split(',')]
            # Language codes are typically 2 letters and start with capital letter
            is_language_code = any(len(p) == 2 and p[0].isupper() for p in parts)
        
        if is_language_code or region_text.startswith('En') or region_text.startswith('Ja'):
            # Look for the previous parenthesis (actual region)
            name_before_lang = name_without_ext[:match.start()].strip()
            prev_match = re.search(region_pattern, name_before_lang)
            if prev_match:
                region_text = prev_match.group(1)
                base_name = name_before_lang[:prev_match.start()].strip()
        
        return base_name, region_text, disc_info
    
    # If no region found, return the full name and "Unknown"
    return name_without_ext, "Unknown", disc_info


def categorize_language(region: str) -> str:
    """
    Categorize a region into language groups: English, Japanese, or Other.
    
    Handles multi-region strings like "USA, Europe" or "USA, Australia".
    """
    region_upper = region.upper()
    
    # English-speaking regions and multi-language with English
    english_regions = {
        'USA', 'US', 'WORLD', 'EUROPE', 'EU', 
        'UNITED KINGDOM', 'UK', 'CANADA', 'AUSTRALIA',
    }
    
    # Check for multi-language releases that include English (e.g., "En,Fr,De")
    if 'EN' in region_upper and ',' in region:
        return 'English'
    
    # Check for multi-region releases (e.g., "USA, Europe")
    if ',' in region:
        # Split by comma and check if ANY region is English
        regions = [r.strip() for r in region.split(',')]
        for r in regions:
            if r in english_regions or r.upper() in english_regions:
                return 'English'
        # If none are English, check if any are Japanese
        japanese_regions = {'JAPAN', 'JP', 'ASIA'}
        for r in regions:
            if r in japanese_regions or r.upper() in japanese_regions:
                return 'Japanese'
        # Otherwise, it's Other
        return 'Other'
    
    # Single region check
    if region in english_regions or region_upper in english_regions:
        return 'English'
    
    # Japanese/Asian regions
    japanese_regions = {'JAPAN', 'JP', 'ASIA'}
    if region in japanese_regions or region_upper in japanese_regions:
        return 'Japanese'
    
    # Everything else (Korea, China, single-language European, etc.)
    return 'Other'


def get_language_priority(region: str, language_group: str) -> int:
    """
    Get priority within a language group. Lower number = higher priority.
    
    Handles multi-region strings like "USA, Europe" by using the highest priority region.
    """
    region_upper = region.upper()
    
    if language_group == 'English':
        # English priority: USA > World > Europe > UK > Canada > Australia
        priority_map = {
            'USA': 0, 'US': 0,
            'WORLD': 1,
            'EUROPE': 2, 'EU': 2,
            'UNITED KINGDOM': 3, 'UK': 3,
            'CANADA': 4,
            'AUSTRALIA': 5,
        }
        
        # Handle multi-region (e.g., "USA, Europe") - use highest priority
        if ',' in region:
            regions = [r.strip() for r in region.split(',')]
            priorities = [priority_map.get(r.upper(), priority_map.get(r, 10)) for r in regions]
            return min(priorities) if priorities else 10
        
        return priority_map.get(region_upper, priority_map.get(region, 10))
    
    elif language_group == 'Japanese':
        # Japanese priority: Japan > Asia
        priority_map = {
            'JAPAN': 0, 'JP': 0,
            'ASIA': 1,
        }
        
        # Handle multi-region
        if ',' in region:
            regions = [r.strip() for r in region.split(',')]
            priorities = [priority_map.get(r.upper(), priority_map.get(r, 10)) for r in regions]
            return min(priorities) if priorities else 10
        
        return priority_map.get(region_upper, priority_map.get(region, 10))
    
    else:  # Other
        # For "Other" languages, prioritize by region
        priority_map = {
            'KOREA': 0,
            'CHINA': 1,
            'SPAIN': 2,
            'FRANCE': 3,
            'GERMANY': 4,
            'ITALY': 5,
            'NETHERLANDS': 6,
        }
        
        # Handle multi-region
        if ',' in region:
            regions = [r.strip() for r in region.split(',')]
            priorities = [priority_map.get(r.upper(), priority_map.get(r, 10)) for r in regions]
            return min(priorities) if priorities else 10
        
        return priority_map.get(region_upper, priority_map.get(region, 10))


def extract_revision_number(game_name: str) -> int:
    """
    Extract revision number from game name. Higher is better.
    Returns -1 for base version (no revision), 0+ for (Rev X).
    """
    match = re.search(r'\(Rev (\d+)\)', game_name)
    if match:
        return int(match.group(1))
    return -1


def is_demo_or_kiosk(game_name: str) -> bool:
    """
    Check if a game is a demo or kiosk version that should be excluded.
    Returns True if the game should be excluded from 1G1R selection.
    """
    name_lower = game_name.lower()
    
    # Check for demo/kiosk/beta/proto/cheat indicators
    demo_indicators = [
        '(demo)',
        '(kiosk)',
        '(preview)',
        '(trial)',
        '(sample)',
        'demo)',  # Catch cases where there might be other tags before (Demo)
        'kiosk)', # Catch cases where there might be other tags before (Kiosk)
        'wi-fi kiosk',  # Wi-Fi kiosk variants
        'wifi kiosk',   # Alternative spelling
        'download station',  # DS Download Station demos
        'distribution',  # Distribution/save data kiosks
        'save data',     # Kiosk save data files
        '(bonus disc)',  # Bonus discs (not the main game)
        'bonus disc)',   # Catch cases with other tags before
        '(debug)',       # Debug versions
        'debug)',        # Catch cases with other tags before
        '(beta)',        # Beta versions
        'beta)',         # Catch cases with other tags before
        '(proto)',       # Prototype versions
        'proto)',        # Catch cases with other tags before
        'sample)',       # Sample versions (different from (sample))
        'jitsuen-you sample',  # Japanese sample versions
    ]
    
    # Check for Action Replay / cheat discs (unlicensed)
    cheat_indicators = [
        'action replay',
        'gameshark',
        'codebreaker',
        'cheat',
        'ultimate cheats',
        'ultimate codes',
        'karat gc-you',
        'pro action replay',
    ]
    
    # Additional check: if it contains "station" and "demo" together
    if 'station' in name_lower and 'demo' in name_lower:
        return True
    
    # Additional check: if it contains "kiosk" anywhere (more comprehensive)
    if 'kiosk' in name_lower:
        return True
    
    # Check for unlicensed (Unl) cheat tools
    if '(unl)' in name_lower and any(cheat in name_lower for cheat in cheat_indicators):
        return True
    
    return any(indicator in name_lower for indicator in demo_indicators)


def filter_by_language(games: List[Dict[str, Any]]) -> Dict[str, List[Dict[str, Any]]]:
    """
    Filter games into three language-based lists: English, Japanese, and Other.
    Each game appears in exactly one list.
    Prefers highest revision numbers (bug fixes/improvements).
    Preserves all discs for multi-disc games.
    """
    # Group games by base name AND disc info
    # This ensures each disc is treated separately
    games_by_base_and_disc = defaultdict(list)
    
    for game in games:
        base_name, region, disc_info = extract_base_name_and_region(game['name'])
        language = categorize_language(region)
        revision = extract_revision_number(game['name'])
        
        # Key includes disc info so each disc is treated separately
        # e.g., ("Final Fantasy VII", "Disc 1") and ("Final Fantasy VII", "Disc 2") are different keys
        key = (base_name, disc_info)
        
        games_by_base_and_disc[key].append({
            'game': game,
            'base_name': base_name,
            'region': region,
            'language': language,
            'revision': revision,
            'disc_info': disc_info
        })
    
    # Categorize and select one game per base name + disc combo
    english_games = []
    japanese_games = []
    other_games = []
    
    for (base_name, disc_info), game_variants in games_by_base_and_disc.items():
        # Check which languages are available
        available_languages = {v['language'] for v in game_variants}
        
        # Decide which language list this game belongs to
        if 'English' in available_languages:
            target_language = 'English'
            target_list = english_games
        elif 'Japanese' in available_languages:
            target_language = 'Japanese'
            target_list = japanese_games
        else:
            target_language = 'Other'
            target_list = other_games
        
        # Filter to only variants in the target language
        language_variants = [v for v in game_variants if v['language'] == target_language]
        
        # Sort by:
        # 1. Region priority (USA > World > Europe, etc.)
        # 2. Revision number (higher is better, so negate for ascending sort)
        # 3. Name for consistency
        language_variants.sort(key=lambda x: (
            get_language_priority(x['region'], target_language),
            -x['revision'],  # Negative so higher revisions come first
            x['game']['name']
        ))
        
        # Select the first one (highest priority)
        selected = language_variants[0]
        target_list.append(selected['game'])
    
    # Sort each list by name for clean output
    english_games.sort(key=lambda x: x['name'])
    japanese_games.sort(key=lambda x: x['name'])
    other_games.sort(key=lambda x: x['name'])
    
    return {
        'english': english_games,
        'japanese': japanese_games,
        'other': other_games
    }


def main():
    parser = argparse.ArgumentParser(
        description='Filter game list to 1G1R (1 Game 1 ROM) organized by language.'
    )
    parser.add_argument(
        'input_file',
        help='Input JSON file containing game list'
    )
    parser.add_argument(
        '-o', '--output-dir',
        default='.',
        help='Output directory for filtered JSON files (default: current directory)'
    )
    parser.add_argument(
        '-s', '--stats',
        action='store_true',
        help='Print detailed statistics about filtering'
    )
    
    args = parser.parse_args()
    
    # Load input JSON
    print(f"Loading games from {args.input_file}...")
    with open(args.input_file, 'r', encoding='utf-8') as f:
        games = json.load(f)
    
    original_count = len(games)
    print(f"Total games in input: {original_count}")
    
    # Exclude demos and kiosk versions from consideration
    print("Excluding demo and kiosk versions...")
    games_before_demo_filter = len(games)
    games = [game for game in games if not is_demo_or_kiosk(game['name'])]
    demos_excluded = games_before_demo_filter - len(games)
    print(f"Excluded {demos_excluded} demo/kiosk versions")
    print(f"Remaining games for filtering: {len(games)}")
    print()
    
    # Filter games by language
    print("Filtering games by language...")
    filtered_by_language = filter_by_language(games)
    
    english_games = filtered_by_language['english']
    japanese_games = filtered_by_language['japanese']
    other_games = filtered_by_language['other']
    
    total_filtered = len(english_games) + len(japanese_games) + len(other_games)
    
    print(f"+ English games:   {len(english_games):4d}")
    print(f"+ Japanese games:  {len(japanese_games):4d}")
    print(f"+ Other languages: {len(other_games):4d}")
    print(f"{'-' * 30}")
    print(f"  Total unique:    {total_filtered:4d}")
    print(f"  Duplicates removed: {original_count - total_filtered}")
    print()
    
    # Save output JSON files
    import os
    output_dir = args.output_dir
    
    files = [
        ('games_1g1r_english.json', english_games, 'English'),
        ('games_1g1r_japanese.json', japanese_games, 'Japanese'),
        ('games_1g1r_other.json', other_games, 'Other languages')
    ]
    
    print("Saving filtered lists...")
    for filename, game_list, description in files:
        output_path = os.path.join(output_dir, filename)
        with open(output_path, 'w', encoding='utf-8') as f:
            json.dump(game_list, f, indent=2, ensure_ascii=False)
        print(f"  + {filename} ({len(game_list)} games)")
    
    print()
    print("Successfully created language-based 1G1R lists!")
    
    # Print statistics if requested
    if args.stats:
        print()
        print("=" * 60)
        print("DETAILED STATISTICS")
        print("=" * 60)
        
        for language_name, game_list in [('English', english_games), 
                                          ('Japanese', japanese_games), 
                                          ('Other', other_games)]:
            if not game_list:
                continue
                
            print()
            print(f"{language_name} Games ({len(game_list)} total):")
            print("-" * 40)
            
            region_counts = defaultdict(int)
            for game in game_list:
                _, region, _ = extract_base_name_and_region(game['name'])
                region_counts[region] += 1
            
            print("  Region distribution:")
            for region, count in sorted(region_counts.items(), key=lambda x: -x[1])[:10]:
                percentage = (count / len(game_list)) * 100
                print(f"    {region:20s}: {count:4d} ({percentage:.1f}%)")
            
            if len(region_counts) > 10:
                other_count = sum(count for region, count in region_counts.items() 
                                 if region not in [r for r, _ in sorted(region_counts.items(), key=lambda x: -x[1])[:10]])
                print(f"    {'(others)':20s}: {other_count:4d}")
        
        print()
        print("=" * 60)


if __name__ == '__main__':
    main()

