#!/usr/bin/env python3
"""
Create 1G1R sets from Myrient file listings.
Fetches HTML from Myrient, parses it, and creates 1G1R JSON files.
"""

import json
import re
import requests
from bs4 import BeautifulSoup
from urllib.parse import unquote, quote
from collections import defaultdict
from typing import List, Dict, Any, Tuple
import argparse
import sys


def fetch_myrient_listing(url: str) -> str:
    """Fetch the file listing HTML from Myrient."""
    print(f"Fetching: {url}")
    headers = {
        'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
    }
    response = requests.get(url, headers=headers, timeout=60)
    response.raise_for_status()
    return response.text


def parse_myrient_listing(html_content: str, base_url: str) -> List[Dict[str, Any]]:
    """Parse Myrient HTML listing and extract file information."""
    soup = BeautifulSoup(html_content, 'html.parser')
    files = []
    
    table = soup.find('table', id='list')
    if table:
        tbody = table.find('tbody')
        if tbody:
            for row in tbody.find_all('tr'):
                cells = row.find_all('td')
                if len(cells) >= 3:
                    name_cell = cells[0]
                    link = name_cell.find('a')
                    
                    if link:
                        file_name = link.get('title', link.text.strip())
                        file_url = link.get('href', '')
                        
                        # Decode URL-encoded characters
                        decoded_name = unquote(file_name)
                        
                        # Extract file size
                        size_text = cells[1].text.strip()
                        file_size = size_text if size_text != '-' else None
                        
                        # Skip directories and navigation
                        is_directory = file_name.endswith('/') or file_name in ['.', '..', 'Parent directory/']
                        
                        # Skip BIOS files
                        if '[BIOS]' in decoded_name:
                            continue
                        
                        if not is_directory and decoded_name.endswith('.zip'):
                            files.append({
                                'name': decoded_name,
                                'url': base_url + quote(decoded_name),
                                'size': file_size
                            })
    
    return files


def extract_base_name_and_region(game_name: str) -> Tuple[str, str, str]:
    """
    Extract the base game name, region, and disc info from the full game name.
    Returns: (base_name, region, disc_info)
    """
    # Remove file extension
    name_without_ext = re.sub(r'\.(zip|7z|rar|chd|cue|bin|iso)$', '', game_name, flags=re.IGNORECASE)
    
    # Extract disc info if present
    disc_info = ""
    disc_pattern = r'\s*\(Disc\s*\d+\)(?:\s*\([^)]*\))?'
    disc_match = re.search(disc_pattern, name_without_ext, re.IGNORECASE)
    if disc_match:
        disc_info = disc_match.group(0).strip()
        name_without_ext = name_without_ext[:disc_match.start()] + name_without_ext[disc_match.end():]
        name_without_ext = name_without_ext.strip()
    
    # Look for region in parentheses
    region_pattern = r'\(([^)]+)\)(?:\s*\([^)]*\))*$'
    match = re.search(region_pattern, name_without_ext)
    
    if match:
        region_text = match.group(1)
        base_name = name_without_ext[:match.start()].strip()
        
        # Check if it's a language code vs region
        is_language_code = False
        if ',' in region_text:
            parts = [p.strip() for p in region_text.split(',')]
            is_language_code = any(len(p) == 2 and p[0].isupper() for p in parts)
        
        if is_language_code or region_text.startswith('En') or region_text.startswith('Ja'):
            name_before_lang = name_without_ext[:match.start()].strip()
            prev_match = re.search(region_pattern, name_before_lang)
            if prev_match:
                region_text = prev_match.group(1)
                base_name = name_before_lang[:prev_match.start()].strip()
        
        return base_name, region_text, disc_info
    
    return name_without_ext, "Unknown", disc_info


def categorize_language(region: str) -> str:
    """Categorize a region into language groups: English, Japanese, or Other."""
    region_upper = region.upper()
    
    english_regions = {
        'USA', 'US', 'WORLD', 'EUROPE', 'EU',
        'UNITED KINGDOM', 'UK', 'CANADA', 'AUSTRALIA',
    }
    
    # Multi-language with English
    if 'EN' in region_upper and ',' in region:
        return 'English'
    
    # Multi-region
    if ',' in region:
        regions = [r.strip() for r in region.split(',')]
        for r in regions:
            if r.upper() in english_regions:
                return 'English'
        japanese_regions = {'JAPAN', 'JP', 'ASIA'}
        for r in regions:
            if r.upper() in japanese_regions:
                return 'Japanese'
        return 'Other'
    
    if region_upper in english_regions:
        return 'English'
    
    japanese_regions = {'JAPAN', 'JP', 'ASIA'}
    if region_upper in japanese_regions:
        return 'Japanese'
    
    return 'Other'


def get_language_priority(region: str, language_group: str) -> int:
    """Get priority within a language group. Lower = higher priority."""
    region_upper = region.upper()
    
    if language_group == 'English':
        priority_map = {
            'USA': 0, 'US': 0, 'WORLD': 1,
            'EUROPE': 2, 'EU': 2,
            'UNITED KINGDOM': 3, 'UK': 3,
            'CANADA': 4, 'AUSTRALIA': 5,
        }
        if ',' in region:
            regions = [r.strip() for r in region.split(',')]
            priorities = [priority_map.get(r.upper(), 10) for r in regions]
            return min(priorities) if priorities else 10
        return priority_map.get(region_upper, 10)
    
    elif language_group == 'Japanese':
        priority_map = {'JAPAN': 0, 'JP': 0, 'ASIA': 1}
        return priority_map.get(region_upper, 10)
    
    return 10


def is_bad_dump(game_name: str) -> bool:
    """Check if the game is marked as a bad dump or unwanted."""
    bad_markers = ['(Beta)', '(Proto)', '(Demo)', '(Sample)', '(Promo)', '(Rev ']
    return any(marker in game_name for marker in bad_markers)


def filter_1g1r(games: List[Dict], prefer_clean: bool = True) -> Dict[str, List[Dict]]:
    """
    Filter games to 1G1R, organized by language.
    Returns dict with 'english', 'japanese', 'other' keys.
    """
    # Group games by base name
    game_groups = defaultdict(list)
    
    for game in games:
        base_name, region, disc_info = extract_base_name_and_region(game['name'])
        language = categorize_language(region)
        priority = get_language_priority(region, language)
        is_bad = is_bad_dump(game['name'])
        
        game_groups[base_name].append({
            'game': game,
            'region': region,
            'disc_info': disc_info,
            'language': language,
            'priority': priority,
            'is_bad': is_bad
        })
    
    # Select best version for each language
    results = {'english': [], 'japanese': [], 'other': []}
    
    for base_name, variants in game_groups.items():
        # Group by language
        by_language = defaultdict(list)
        for v in variants:
            by_language[v['language']].append(v)
        
        for language, lang_variants in by_language.items():
            # Sort: prefer non-bad dumps, then by priority
            if prefer_clean:
                lang_variants.sort(key=lambda x: (x['is_bad'], x['priority']))
            else:
                lang_variants.sort(key=lambda x: x['priority'])
            
            # Get best version (may have multiple discs)
            best = lang_variants[0]
            
            # Check for multi-disc games
            if best['disc_info']:
                # Find all discs for this game version
                same_version = [v for v in lang_variants 
                               if v['region'] == best['region'] and v['is_bad'] == best['is_bad']]
                for v in same_version:
                    results[language.lower()].append(v['game'])
            else:
                results[language.lower()].append(best['game'])
    
    # Sort results alphabetically
    for lang in results:
        results[lang].sort(key=lambda x: x['name'].lower())
    
    return results


def create_1g1r_set(system_name: str, myrient_url: str, output_prefix: str):
    """Create 1G1R JSON files for a system from Myrient."""
    print(f"\n{'='*60}")
    print(f"Creating 1G1R set for: {system_name}")
    print(f"{'='*60}")
    
    # Fetch and parse
    html = fetch_myrient_listing(myrient_url)
    games = parse_myrient_listing(html, myrient_url)
    
    print(f"Found {len(games)} total games")
    
    # Filter to 1G1R
    results = filter_1g1r(games)
    
    # Save JSON files
    for lang in ['english', 'japanese', 'other']:
        if results[lang]:
            filename = f"1g1rsets/games_1g1r_{lang}_{output_prefix}.json"
            output_data = {
                "system": system_name,
                "source": myrient_url,
                "total_games": len(results[lang]),
                "games": results[lang]
            }
            with open(filename, 'w', encoding='utf-8') as f:
                json.dump(output_data, f, indent=2, ensure_ascii=False)
            print(f"  Saved {len(results[lang])} {lang} games to {filename}")
    
    # Summary
    total = sum(len(results[l]) for l in results)
    print(f"\nTotal 1G1R games: {total}")
    print(f"  English: {len(results['english'])}")
    print(f"  Japanese: {len(results['japanese'])}")
    print(f"  Other: {len(results['other'])}")


def main():
    parser = argparse.ArgumentParser(description='Create 1G1R sets from Myrient')
    parser.add_argument('--system', choices=['ngp', 'ngpc', 'saturn', 'all'], 
                       default='all', help='System to process')
    args = parser.parse_args()
    
    systems = {
        'ngp': {
            'name': 'Neo Geo Pocket',
            'url': 'https://myrient.erista.me/files/No-Intro/SNK%20-%20NeoGeo%20Pocket/',
            'prefix': 'ngp'
        },
        'ngpc': {
            'name': 'Neo Geo Pocket Color',
            'url': 'https://myrient.erista.me/files/No-Intro/SNK%20-%20NeoGeo%20Pocket%20Color/',
            'prefix': 'ngpc'
        },
        'saturn': {
            'name': 'Sega Saturn',
            'url': 'https://myrient.erista.me/files/Redump/Sega%20-%20Saturn/',
            'prefix': 'saturn'
        }
    }
    
    if args.system == 'all':
        for sys_id, sys_info in systems.items():
            create_1g1r_set(sys_info['name'], sys_info['url'], sys_info['prefix'])
    else:
        sys_info = systems[args.system]
        create_1g1r_set(sys_info['name'], sys_info['url'], sys_info['prefix'])


if __name__ == '__main__':
    main()
